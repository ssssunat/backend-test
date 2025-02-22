# Backend developer test
=================

## Project Description
Write notification service to notify user for card activities on his card. You need to write HTTP server which accepts events in JSON format via POST method and stores them in some sort of storage 
(could store either in db or some basic memory storage). You also need to write worker/job which will notify those events to client. Notification can be mocked just by printing them to terminal.

##Getting started
### Requirements
- **Go**
- **PostgreSQL**

##Installation and Run
1. Clone the repostiory:
    ```bash
    git clone https://github.com/spatecon/cartevent.git
    cd cartevent
    ```
2. Download dependencies:
    ```bash
    go mod download
    ```

3. Start the server:
    ```bash
    export DATABASE_URL="postgres://hts-user:hts-pass@localhost:5432/hts?sslmode=disable"
    go run main.go
    ```

## API Endpoints
- `POST /api/v1/event` — create a new event.

### Request Examples

#### Create a Booking
```bash
curl --request POST \
  --url http://localhost:8080/api/v1/event \
  --header 'content-type: application/json' \
  --data {
  "orderType": "Purchase",
  "sessionId": "29827525-06c9-4b1e-9d9b-7c4584e82f56",
  "card": "4433**1409",
  "eventDate": "2023-01-04 13:44:52.835626 +00:00",
  "websiteUrl": "https://amazon.com"
}
```
