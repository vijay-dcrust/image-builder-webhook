package main

import (
	"io"
	"log"
	"net/http"

	"imgbuilder/v1/admissioncontrol"
)

func main() {

	http.HandleFunc("/", ExampleHandler)
	http.HandleFunc("/mutate", admissioncontrol.WebhookMutator)
	log.Println("** Service Started on Port 3000 **")

	// Use ListenAndServeTLS() instead of ListenAndServe() which accepts two extra parameters.
	// We need to specify both the certificate file and the key file (which we've named
	// https-server.crt and https-server.key).
	err := http.ListenAndServeTLS(":3000", "img-builder.crt", "img-builder.key", nil)
	if err != nil {
		log.Fatal(err)
	}
}

// ExampleHandler Default path handler function
func ExampleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	io.WriteString(w, `{"status":"ok"}`)
}
