package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	amqp "github.com/rabbitmq/amqp091-go"
)

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	//connect to database, writes go to the primary and reads to the replicas
	db, err := sql.Open("mysql", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	readDB := db
	if readURL := os.Getenv("DATABASE_READ_URL"); readURL != "" {
		readDB, err = sql.Open("mysql", readURL)
		if err != nil {
			log.Fatal(err)
		}
		defer readDB.Close()
	}

	//create the table if it doesn't exist
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS users (id INT AUTO_INCREMENT PRIMARY KEY, name TEXT, email TEXT)")

	if err != nil {
		log.Fatal(err)
	}

	//connect to rabbitmq, separate channels for publishing and consuming
	rabbit := connectRabbit(os.Getenv("RABBITMQ_URL"))
	defer rabbit.Close()

	publishCh := openChannel(rabbit)
	consumeCh := openChannel(rabbit)
	go consumeUsers(consumeCh, db)

	//create router
	router := mux.NewRouter()
	router.HandleFunc("/hello", getHelloWorld(readDB)).Methods("GET")
	router.HandleFunc("/users", getUsers(readDB)).Methods("GET")
	router.HandleFunc("/users/{id}", getUser(readDB)).Methods("GET")
	router.HandleFunc("/users", createUser(publishCh)).Methods("POST")
	router.HandleFunc("/users/{id}", updateUser(db)).Methods("PUT")
	router.HandleFunc("/users/{id}", deleteUser(db)).Methods("DELETE")

	//start server
	log.Fatal(http.ListenAndServe(":8080", jsonContentTypeMiddleware(router)))
}

func getHelloWorld(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode("Hello World")
	})
}

func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// get all users
func getUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT * FROM users")
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()

		users := []User{}
		for rows.Next() {
			var u User
			if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
				log.Fatal(err)
			}
			users = append(users, u)
		}
		if err := rows.Err(); err != nil {
			log.Fatal(err)
		}

		json.NewEncoder(w).Encode(users)
	}
}

// get user by id
func getUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u User
		err := db.QueryRow("SELECT * FROM users WHERE id = ?", id).Scan(&u.ID, &u.Name, &u.Email)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(u)
	}
}

// create user, queued and saved to the database by the consumer
func createUser(ch *amqp.Channel) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u User
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := publishUser(ch, u); err != nil {
			log.Printf("publish failed: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode("User queued")
	}
}

// update user
func updateUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u User
		json.NewDecoder(r.Body).Decode(&u)

		vars := mux.Vars(r)
		id := vars["id"]

		_, err := db.Exec("UPDATE users SET name = ?, email = ? WHERE id = ?", u.Name, u.Email, id)
		if err != nil {
			log.Fatal(err)
		}

		json.NewEncoder(w).Encode(u)
	}
}

// delete user
func deleteUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u User
		err := db.QueryRow("SELECT * FROM users WHERE id = ?", id).Scan(&u.ID, &u.Name, &u.Email)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			_, err := db.Exec("DELETE FROM users WHERE id = ?", id)
			if err != nil {
				//todo : fix error handling
				w.WriteHeader(http.StatusNotFound)
				return
			}

			json.NewEncoder(w).Encode("User deleted")
		}
	}
}
