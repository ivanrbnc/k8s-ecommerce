package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/redis/go-redis/v9"
)

type CartItem struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

type Cart struct {
	UserID string     `json:"user_id"`
	Items  []CartItem `json:"items"`
}

var (
	rdb *redis.Client
	ctx = context.Background()
)

func main() {
	// Connect to Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	rdb = redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   0,
	})

	// Test Redis connection
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully")

	router := mux.NewRouter()

	router.HandleFunc("/health", healthHandler).Methods("GET")
	router.HandleFunc("/cart/{userId}", getCartHandler).Methods("GET")
	router.HandleFunc("/cart/{userId}/add", addToCartHandler).Methods("POST")
	router.HandleFunc("/cart/{userId}/remove", removeFromCartHandler).Methods("POST")
	router.HandleFunc("/cart/{userId}/clear", clearCartHandler).Methods("DELETE")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8002"
	}

	log.Printf("Cart Service starting on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check Redis connection
	_, err := rdb.Ping(ctx).Result()
	status := "healthy"
	if err != nil {
		status = "unhealthy"
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status":  status,
		"service": "cart-service",
	})
}

func getCartHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	userID := vars["userId"]

	cartData, err := rdb.Get(ctx, "cart:"+userID).Result()
	if err == redis.Nil {
		// Cart doesn't exist, return empty cart
		json.NewEncoder(w).Encode(Cart{UserID: userID, Items: []CartItem{}})
		return
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to retrieve cart"})
		return
	}

	var cart Cart
	if err := json.Unmarshal([]byte(cartData), &cart); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse cart"})
		return
	}

	json.NewEncoder(w).Encode(cart)
}

func addToCartHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	userID := vars["userId"]

	var newItem CartItem
	if err := json.NewDecoder(r.Body).Decode(&newItem); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Get existing cart
	var cart Cart
	cartData, err := rdb.Get(ctx, "cart:"+userID).Result()
	if err == redis.Nil {
		cart = Cart{UserID: userID, Items: []CartItem{}}
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to retrieve cart"})
		return
	} else {
		if err := json.Unmarshal([]byte(cartData), &cart); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse cart"})
			return
		}
	}

	// Check if item already exists in cart
	found := false
	for i, item := range cart.Items {
		if item.ProductID == newItem.ProductID {
			cart.Items[i].Quantity += newItem.Quantity
			found = true
			break
		}
	}

	if !found {
		cart.Items = append(cart.Items, newItem)
	}

	// Save cart back to Redis
	cartJSON, err := json.Marshal(cart)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save cart"})
		return
	}

	if err := rdb.Set(ctx, "cart:"+userID, cartJSON, 0).Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save cart"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cart)
}

func removeFromCartHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	userID := vars["userId"]

	var removeItem struct {
		ProductID int `json:"product_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&removeItem); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Get existing cart
	cartData, err := rdb.Get(ctx, "cart:"+userID).Result()
	if err == redis.Nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Cart not found"})
		return
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to retrieve cart"})
		return
	}

	var cart Cart
	if err := json.Unmarshal([]byte(cartData), &cart); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse cart"})
		return
	}

	// Remove item from cart
	newItems := []CartItem{}
	for _, item := range cart.Items {
		if item.ProductID != removeItem.ProductID {
			newItems = append(newItems, item)
		}
	}
	cart.Items = newItems

	// Save cart back to Redis
	cartJSON, err := json.Marshal(cart)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save cart"})
		return
	}

	if err := rdb.Set(ctx, "cart:"+userID, cartJSON, 0).Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save cart"})
		return
	}

	json.NewEncoder(w).Encode(cart)
}

func clearCartHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	userID := vars["userId"]

	if err := rdb.Del(ctx, "cart:"+userID).Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to clear cart"})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Cart cleared successfully"})
}
