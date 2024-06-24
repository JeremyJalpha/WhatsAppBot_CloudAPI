package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"database/sql"

	wb "github.com/JeremyJalpha/WhatsAppBot/whatsappbot"
	"github.com/febriliankr/whatsapp-cloud-api"
	"github.com/go-chi/chi/v5"

	_ "github.com/lib/pq"
)

const (
	webhookURL          = "/webhook"
	staleMsgTimeOut int = 10
	pymntRtrnBase       = "payment_return"
	pymntCnclBase       = "payment_canceled"
	returnBaseURL       = "/" + pymntRtrnBase
	cancelBaseURL       = "/" + pymntCnclBase
	notifyBaseURL       = "/payment_notify"
	ItemNamePrefix      = "Order"
	isAutoInc           = false
)

type EnvVars struct {
	Port          string
	VerifyToken   string
	WhatsAppToken string
	DBConn        string
	HostNumber    string
	PhoneID       string
	HomebaseURL   string
	MerchantId    string
	MerchantKey   string
	Passphrase    string
	PfHost        string
}

func getEnvVar(name string) string {
	value, exists := os.LookupEnv(name)
	if !exists {
		log.Fatalf("%s environment variable does not exist", name)
	}
	return value
}

// TODO: if WhatsApp token is stale app just exits silently without error or warning - please fix.
func main() {
	envVars := EnvVars{
		Port:          getEnvVar("PORT"),
		VerifyToken:   getEnvVar("VERIFY_TOKEN"),
		WhatsAppToken: getEnvVar("WHATSAPP_TOKEN"),
		DBConn:        getEnvVar("DATABASE_URL"),
		HostNumber:    getEnvVar("HOST_NUMBER"),
		PhoneID:       getEnvVar("PHONE_ID"),
		HomebaseURL:   getEnvVar("HOMEBASEURL"),
		MerchantId:    getEnvVar("MERCHANTID"),
		MerchantKey:   getEnvVar("MERCHANTKEY"),
		Passphrase:    getEnvVar("PASSPHRASE"),
		PfHost:        getEnvVar("PFHOST"),
	}
	log.Println("Using DB connection string: " + envVars.DBConn)

	bgCtx := context.Background()
	_, cancel := context.WithTimeout(bgCtx, 10*time.Second)
	defer cancel()

	//Initialize a new WhatsApp instance
	wa := whatsapp.NewWhatsapp(envVars.WhatsAppToken, envVars.PhoneID)

	// Open the database connection
	db, err := sql.Open("postgres", envVars.DBConn)
	if err != nil {
		log.Fatal("Error opening database: ", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatal("Error closing database: ", err)
		}
	}()

	// Get the current working directory
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal("Failed to get current directory: ", err)
	}

	// Construct the path to the template file
	pymntRtrnTplPath := filepath.Join(pwd, "templates", pymntRtrnBase+".html")
	pymntCnclTplPath := filepath.Join(pwd, "templates", pymntCnclBase+".html")

	pymntRtrnTpl := template.Must(template.ParseFiles(pymntRtrnTplPath))
	pymntCnclTpl := template.Must(template.ParseFiles(pymntCnclTplPath))

	r := chi.NewRouter()

	checkoutInfo := wb.CheckoutInfo{
		ReturnURL:      envVars.HomebaseURL + returnBaseURL,
		CancelURL:      envVars.HomebaseURL + cancelBaseURL,
		NotifyURL:      envVars.HomebaseURL + notifyBaseURL,
		MerchantId:     envVars.MerchantId,
		MerchantKey:    envVars.MerchantKey,
		Passphrase:     envVars.Passphrase,
		HostURL:        envVars.PfHost,
		ItemNamePrefix: ItemNamePrefix,
	}

	// Define routes
	chatClient := wb.ChatClient{
		Whatsapp: wa,
	}
	r.Post(webhookURL, WebhookHandler(envVars.VerifyToken, envVars.HostNumber, staleMsgTimeOut, &chatClient, db, checkoutInfo))
	r.Get(webhookURL, VerificationHandler(envVars.VerifyToken))

	// Define other routes
	r.Get(returnBaseURL, PaymentReturnHandler(pymntRtrnTpl))
	r.Get(notifyBaseURL, PaymentNotifyHandler(envVars.Passphrase, envVars.PfHost))
	r.Get(cancelBaseURL, PaymentCancelHandler(pymntCnclTpl))

	if envVars.Port == "" {
		log.Fatalf("Fatal Error Port not set")
	}

	err = http.ListenAndServe(":"+envVars.Port, r)
	if err != nil {
		log.Fatal("Fatal Error serving app: ", err)
	}

	//server := &http.Server{
	//	Addr:    ":" + port,
	//	Handler: r,
	//}

	//var serverErr error
	//serverErr = server.ListenAndServeTLS("path/to/cert.pem", "path/to/key.pem")
	//if err != nil {
	//	log.Fatal(serverErr)
	//}
	log.Println("Server is running on port ", envVars.Port)
}
