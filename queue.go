package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const usersQueue = "users.create"

// connect to rabbitmq, retrying because the broker may start after the app
func connectRabbit(url string) *amqp.Connection {
	for i := 0; i < 30; i++ {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn
		}
		log.Printf("rabbitmq not ready, retrying: %v", err)
		time.Sleep(2 * time.Second)
	}
	log.Fatal("could not connect to rabbitmq")
	return nil
}

// open a channel and make sure the queue exists
func openChannel(conn *amqp.Connection) *amqp.Channel {
	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}

	//durable queue survives broker restarts
	_, err = ch.QueueDeclare(usersQueue, true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	return ch
}

// publish user to the queue
func publishUser(ch *amqp.Channel, u User) error {
	body, err := json.Marshal(u)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return ch.PublishWithContext(ctx, "", usersQueue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// consume users from the queue and save them to the database
func consumeUsers(ch *amqp.Channel, db *sql.DB) {
	//handle one message at a time
	if err := ch.Qos(1, 0, false); err != nil {
		log.Fatal(err)
	}

	msgs, err := ch.Consume(usersQueue, "", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		var u User
		if err := json.Unmarshal(msg.Body, &u); err != nil {
			log.Printf("invalid message, dropping: %v", err)
			msg.Nack(false, false)
			continue
		}

		err := db.QueryRow("INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id", u.Name, u.Email).Scan(&u.ID)
		if err != nil {
			log.Printf("insert failed, requeueing: %v", err)
			time.Sleep(time.Second)
			msg.Nack(false, true)
			continue
		}

		log.Printf("created user %d from queue", u.ID)
		msg.Ack(false)
	}

	log.Fatal("rabbitmq consumer channel closed")
}
