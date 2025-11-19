package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
)

type Product struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
}

var products = []Product{
	{ID: 1, Name: "Laptop", Description: "High-performance laptop", Price: 999.99, Stock: 10},
	{ID: 2, Name: "Mouse", Description: "Wireless mouse", Price: 29.99, Stock: 50},
	{ID: 3, Name: "Keyboard", Description: "Mechanical keyboard", Price: 79.99, Stock: 30},
	{ID: 4, Name: "Monitor", Description: "4K Monitor", Price: 399.99, Stock: 15},
	{ID: 5, Name: "Headphones", Description: "Noise-cancelling headphones", Price: 199.99, Stock: 25},
}

func main() {
	router := mux.NewRouter()

	router.HandleFunc("/health", healthHandler).Methods("GET")
	router.HandleFunc("/products", getProductsHandler).Methods("GET")
	router.HandleFunc("/products/{id}", getProductByIDHandler).Methods("GET")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8001"
	}

	log.Printf("Product Service starting on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": "product-service"})
}

func getProductsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}

func getProductByIDHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid product ID"})
		return
	}

	for _, product := range products {
		if product.ID == id {
			json.NewEncoder(w).Encode(product)
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "Product not found"})
}
