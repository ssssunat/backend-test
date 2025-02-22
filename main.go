package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	WorkerCount = 8
	Interval    = 1 * time.Second
)

type CartEvent struct {
	OrderType  string `json:"orderType"`
	SessionID  string `json:"sessionId"`
	Card       string `json:"card"`
	EventDate  string `json:"eventDate"`
	WebsiteURL string `json:"websiteUrl"`
}

type PGCartEvent struct {
	ID         string
	OrderType  string
	SessionID  string
	Card       string
	EventDate  time.Time
	WebsiteURL string
	Status     string
	CreatedAt  time.Time
}

func initDB() (*pgxpool.Pool, error) {
	databaseURL := os.Getenv("DATABASE_URL")

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 10
	config.MaxConnIdleTime = 30 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := pgxpool.New(ctx, config.ConnString())
	if err != nil {
		return nil, err
	}

	query := `
	CREATE EXTENSION IF NOT EXISTS "uuid-ossp";	
	CREATE TABLE IF NOT EXISTS cart_events (
		id UUID DEFAULT uuid_generate_v4() PRIMARY KEY,
		order_type varchar(30) not null,
		session_id text not null,
		card varchar(16) not null,
		event_date timestamp not null,
		website_url text not null,
		status varchar(20) DEFAULT 'pending',
		created_at timestamp DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = db.Exec(context.Background(), query)
	if err != nil {
		return nil, err
	}

	return db, nil
}

type Pool struct {
	db         *pgxpool.Pool
	wg         *sync.WaitGroup
	numWorkers int
}

func NewPool(ctx context.Context, numWorkers int, db *pgxpool.Pool) *Pool {
	pool := &Pool{
		db:         db,
		wg:         &sync.WaitGroup{},
		numWorkers: numWorkers,
	}

	pool.wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go pool.workerEvents(ctx)
	}

	return pool
}

func (p *Pool) workerEvents(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			p.process(ctx)
		}
	}
}

func (p *Pool) process(ctx context.Context) {
	time.Sleep(Interval)

	rows, err := p.db.Query(ctx, `
	WITH cte AS (
		SELECT id, order_type, session_id, card, event_date, website_url 
		FROM cart_events 
		WHERE status = 'pending' 
		ORDER BY id 
		LIMIT 10 
		FOR UPDATE SKIP LOCKED
	)
	UPDATE cart_events 
	SET status = 'processing' 
	WHERE id IN (SELECT id FROM cte)
	RETURNING id, order_type, session_id, card, event_date, website_url, status, created_at;
	`)
	defer rows.Close()

	if err != nil {
		log.Println("Error fetching events:", err)
		return
	}

	for rows.Next() {
		var event PGCartEvent

		err := rows.Scan(
			&event.ID,
			&event.OrderType,
			&event.SessionID,
			&event.Card,
			&event.EventDate,
			&event.WebsiteURL,
			&event.Status,
			&event.CreatedAt,
		)
		if err != nil {
			log.Println("Error scanning row:", err)
			continue
		}

		sendNotification(ctx, p.db, event)
	}
}

type Handler struct {
	db *pgxpool.Pool
}

func (h *Handler) Event(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid method",
		})
		return
	}

	var event CartEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid body",
		})
		return
	}

	_, err := h.db.Exec(context.Background(),
		`INSERT INTO cart_events (order_type, session_id, card, event_date, website_url) 
	VALUES ($1, $2, $3, $4, $5)`,
		event.OrderType, event.SessionID, event.Card, event.EventDate, event.WebsiteURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, "event recieved and stored")
}

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatal(err)
	}
	h := &Handler{db: db}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := NewPool(ctx, WorkerCount, db)

	mux := http.NewServeMux()
	mux.HandleFunc("/event", h.Event)

	server := &http.Server{
		Addr: ":8080",
		Handler: mux,
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Println("Shutting down server...")

		cancel()
		pool.wg.Wait()

		ctx1, cancel1 := context.WithTimeout(context.Background(), 10 * time.Second)
		defer cancel1()

		if err := server.Shutdown(ctx1); err != nil {
			log.Println("Error shutting down server:", err)
		}

		db.Close()
		log.Println("Server stopped.")
	}()

	log.Println("HTTP Server runnig on port 8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func sendNotification(ctx context.Context, db *pgxpool.Pool, event PGCartEvent) {
	time.Sleep(2 * time.Second) // Simulate external Notify Service Call
	log.Printf("NOTIFY: Order %s for card %s",
		event.OrderType, event.Card)

	_, err := db.Exec(ctx, "UPDATE cart_events SET status = 'processed' WHERE id = $1", event.ID)
	if err != nil {
		log.Println("Failed to update event status:", err.Error())
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) error {
	w.WriteHeader(status)
	w.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}
