package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type OrderItem struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

type Order struct {
	ID        int         `json:"id"`
	UserID    string      `json:"user_id"`
	Items     []OrderItem `json:"items"`
	Total     float64     `json:"total"`
	Status    string      `json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}

var db *sql.DB

func main() {
	var err error

	// Database connection
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	if dbHost == "" {
		dbHost = "localhost"
	}
	if dbPort == "" {
		dbPort = "5432"
	}
	if dbUser == "" {
		dbUser = "postgres"
	}
	if dbPassword == "" {
		dbPassword = "postgres"
	}
	if dbName == "" {
		dbName = "orders"
	}

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Retry connection logic
	for i := 0; i < 10; i++ {
		db, err = sql.Open("postgres", connStr)
		if err == nil {
			err = db.Ping()
			if err == nil {
				log.Println("Connected to PostgreSQL successfully")
				break
			}
		}
		log.Printf("Failed to connect to database (attempt %d/10): %v", i+1, err)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		log.Fatalf("Could not connect to database after 10 attempts: %v", err)
	}

	// Create tables
	createTables()

	router := mux.NewRouter()

	router.HandleFunc("/health", healthHandler).Methods("GET")
	router.HandleFunc("/orders", createOrderHandler).Methods("POST")
	router.HandleFunc("/orders/{userId}", getUserOrdersHandler).Methods("GET")
	router.HandleFunc("/orders/detail/{orderId}", getOrderDetailHandler).Methods("GET")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8003"
	}

	log.Printf("Order Service starting on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func createTables() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS orders (
			id SERIAL PRIMARY KEY,
			user_id VARCHAR(255) NOT NULL,
			total DECIMAL(10, 2) NOT NULL,
			status VARCHAR(50) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS order_items (
			id SERIAL PRIMARY KEY,
			order_id INTEGER REFERENCES orders(id) ON DELETE CASCADE,
			product_id INTEGER NOT NULL,
			quantity INTEGER NOT NULL
		)`,
	}

	for _, query := range queries {
		_, err := db.Exec(query)
		if err != nil {
			log.Fatalf("Failed to create table: %v", err)
		}
	}

	log.Println("Database tables created successfully")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check database connection
	err := db.Ping()
	status := "healthy"
	if err != nil {
		status = "unhealthy"
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status":  status,
		"service": "order-service",
	})
}

func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var orderRequest struct {
		UserID string      `json:"user_id"`
		Items  []OrderItem `json:"items"`
		Total  float64     `json:"total"`
	}

	if err := json.NewDecoder(r.Body).Decode(&orderRequest); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if orderRequest.UserID == "" || len(orderRequest.Items) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "User ID and items are required"})
		return
	}

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create order"})
		return
	}
	defer tx.Rollback()

	// Insert order
	var orderID int
	err = tx.QueryRow(
		"INSERT INTO orders (user_id, total, status) VALUES ($1, $2, $3) RETURNING id",
		orderRequest.UserID, orderRequest.Total, "pending",
	).Scan(&orderID)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create order"})
		return
	}

	// Insert order items
	for _, item := range orderRequest.Items {
		_, err := tx.Exec(
			"INSERT INTO order_items (order_id, product_id, quantity) VALUES ($1, $2, $3)",
			orderID, item.ProductID, item.Quantity,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create order items"})
			return
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to commit order"})
		return
	}

	order := Order{
		ID:        orderID,
		UserID:    orderRequest.UserID,
		Items:     orderRequest.Items,
		Total:     orderRequest.Total,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(order)
}

func getUserOrdersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	userID := vars["userId"]

	rows, err := db.Query(
		"SELECT id, user_id, total, status, created_at FROM orders WHERE user_id = $1 ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to retrieve orders"})
		return
	}
	defer rows.Close()

	orders := []Order{}
	for rows.Next() {
		var order Order
		err := rows.Scan(&order.ID, &order.UserID, &order.Total, &order.Status, &order.CreatedAt)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse orders"})
			return
		}

		// Get order items
		itemRows, err := db.Query(
			"SELECT product_id, quantity FROM order_items WHERE order_id = $1",
			order.ID,
		)
		if err != nil {
			continue
		}

		items := []OrderItem{}
		for itemRows.Next() {
			var item OrderItem
			itemRows.Scan(&item.ProductID, &item.Quantity)
			items = append(items, item)
		}
		itemRows.Close()

		order.Items = items
		orders = append(orders, order)
	}

	json.NewEncoder(w).Encode(orders)
}

func getOrderDetailHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	orderID := vars["orderId"]

	var order Order
	err := db.QueryRow(
		"SELECT id, user_id, total, status, created_at FROM orders WHERE id = $1",
		orderID,
	).Scan(&order.ID, &order.UserID, &order.Total, &order.Status, &order.CreatedAt)

	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Order not found"})
		return
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to retrieve order"})
		return
	}

	// Get order items
	rows, err := db.Query(
		"SELECT product_id, quantity FROM order_items WHERE order_id = $1",
		order.ID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to retrieve order items"})
		return
	}
	defer rows.Close()

	items := []OrderItem{}
	for rows.Next() {
		var item OrderItem
		rows.Scan(&item.ProductID, &item.Quantity)
		items = append(items, item)
	}

	order.Items = items
	json.NewEncoder(w).Encode(order)
}
