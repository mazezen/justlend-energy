package router

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mazezen/justlend-energy/internel/handle"
)

func NewRouter() *mux.Router {
	router := mux.NewRouter()
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	router.Methods("POST").Path("/fee").HandlerFunc(handle.Fee)
	router.Methods("POST").Path("/rent").HandlerFunc(handle.Rental)
	router.Methods("POST").Path("/return/by/renter").HandlerFunc(handle.ReturnByRenter)
	router.Methods("POST").Path("/return/by/receiver").HandlerFunc(handle.ReturnByReceiver)
	router.Methods("GET").Path("/orderInfo/{renter}/{receiver}/{resourceType:[0-1]+}").HandlerFunc(handle.GetOrderInfo)

	return router
}
