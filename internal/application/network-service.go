package application

import (
	"encoding/json"
	"net/http"
)


type NetworkService struct {
	service *NetworkService
}

func NewNetworkService() *NetworkService {
	return &NetworkService{}

}


// type DocsHandler struct {
// 	service *application.DocsService
// }

// func NewDocsHandler(service *application.DocsService) *DocsHandler {
// 	return &DocsHandler{service: service}
// }





func (s *NetworkService) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
