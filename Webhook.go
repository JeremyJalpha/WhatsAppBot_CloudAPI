package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	wb "github.com/JeremyJalpha/WhatsAppBot/whatsappbot"
)

type StatusesWebhookRequest struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Statuses []struct {
					ID           string `json:"id"`
					Status       string `json:"status"`
					Timestamp    string `json:"timestamp"`
					RecipientID  string `json:"recipient_id"`
					Conversation struct {
						ID                  string `json:"id"`
						ExpirationTimestamp string `json:"expiration_timestamp"`
						Origin              struct {
							Type string `json:"type"`
						} `json:"origin"`
					} `json:"conversation"`
					Pricing struct {
						Billable     bool   `json:"billable"`
						PricingModel string `json:"pricing_model"`
						Category     string `json:"category"`
					} `json:"pricing"`
				} `json:"statuses"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

type ContactsWebhookRequest struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
					Type string `json:"type"`
				} `json:"messages"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

// VerificationHandler handles the GET /webhook route for verification
func VerificationHandler(verifyToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profferedToken := r.URL.Query().Get("hub.verify_token")
		challenge := r.URL.Query().Get("hub.challenge")

		if profferedToken == verifyToken {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(challenge))
			if err != nil {
				http.Error(w, "Internal Server Error.", http.StatusInternalServerError)
				log.Println(err)
				return
			}
			log.Println("Webhook verified.")
		} else {
			err := "Error, wrong validation token."
			w.WriteHeader(http.StatusForbidden)
			_, sendErr := w.Write([]byte(err))
			if sendErr != nil {
				http.Error(w, "Internal Server Error.", http.StatusInternalServerError)
				log.Println(sendErr)
				return
			}
			log.Println(err)
		}
	}
}

// CalculateSignature calculates the signature for the Facebook webhook payload.
func CalculateSignatureSha256(payload, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	rawHmac := mac.Sum(nil)
	return hex.EncodeToString(rawHmac)
}

// Taken from: https://stackoverflow.com/questions/38353831/facebook-webhook-signature-calculation-c
func EscapeNonASCIICharacters(s string) string {
	var escaped string
	for _, c := range s {
		if c > 127 {
			escaped += fmt.Sprintf("\\u%04X", unicode.ToUpper(c))
		} else {
			escaped += string(c)
		}
	}
	return escaped
}

// Checks whether the message is older than the parmater staleMsg in minutes
func IsMessageStale(timestamp string, staleMsg int) bool {

	// Try to parse the string into an int64
	timeInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return true
	}

	// Convert the Unix timestamp to a time.Time value
	now := time.Now() // Add this line to define 'now'
	return now.Sub(time.Unix(timeInt, 0)) >= time.Duration(staleMsg)*time.Minute
}

// IsMessageValid returns the body of the last message
// returns lastMesasgeBody, lastMsgTimeStamp, error
func IsMessageValid(req ContactsWebhookRequest, staleMsg int) (string, string, error) {
	lastEntry := req.Entry[len(req.Entry)-1]
	lastChange := lastEntry.Changes[len(lastEntry.Changes)-1]
	lastMessage := lastChange.Value.Messages[len(lastChange.Value.Messages)-1]
	lastMsgTimeStamp := lastMessage.Timestamp
	MessageBody := strings.ToLower(lastMessage.Text.Body)
	recipientNum := lastMessage.From

	if lastMsgTimeStamp == "" || lastMsgTimeStamp == "-1" {
		return "Err:FailedToGetLastMsgTimeStamp", "-1", fmt.Errorf("error failed to get last message timestamp")
	}
	if IsMessageStale(lastMsgTimeStamp, staleMsg) {
		return "Err:StaleMessage", "-1", fmt.Errorf("error message was stale")
	}
	return MessageBody, recipientNum, nil
}

// WebhookHandler handles the POST /webhook route
func WebhookHandler(appSecret, hostNumber string, staleMsg int, c *wb.ChatClient, db *sql.DB, checkoutUrls wb.CheckoutInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error

		// Verify signature
		signature256 := strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
		if signature256 == "" {
			err := "error, signature is missing"
			http.Error(w, err, http.StatusForbidden)
			log.Println(err)
			return
		}

		// Read the request body
		byteBody, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "error reading request body.", http.StatusInternalServerError)
			log.Println("error reading request body: " + err.Error())
			return
		}

		calculatedSignature256 := CalculateSignatureSha256([]byte(EscapeNonASCIICharacters(string(byteBody))), []byte(appSecret))
		if subtle.ConstantTimeCompare([]byte(calculatedSignature256), []byte(signature256)) != 1 {
			err := "error signatures do not match"
			http.Error(w, err, http.StatusForbidden)
			log.Println(err + "\nExpected Sha256: " + signature256 + "\nbut got Sha256: " + calculatedSignature256)
			return
		}

		// Respond to the webhook request
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("Success"))
		if err != nil {
			log.Println("error writing response: ", err)
		}

		bodyStr := string(byteBody)
		if strings.Contains(bodyStr, "\"statuses\":[{\"id\":\"") {
			log.Println("Status updates unhandled at this time.")
			return
		}

		// Parse the request body from the JSON string
		var req ContactsWebhookRequest
		err = json.Unmarshal(byteBody, &req)
		if err != nil {
			log.Println("error parsing JSON: ", err)
			return
		}

		messageBody, senderNumber, err := IsMessageValid(req, staleMsg)
		if err != nil {
			log.Println("Message was invalid: " + err.Error())
			return
		}

		if senderNumber != hostNumber {
			convo := wb.NewConversationContext(db, senderNumber, messageBody, isAutoInc)
			c.ChatBegin(*convo, db, checkoutUrls, isAutoInc)
		} else {
			log.Println("You sent a message:", messageBody)
		}
	}
}
