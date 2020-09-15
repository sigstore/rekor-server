/*
Copyright © 2020 Luke Hinds <lhinds@redhat.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

// curl http://localhost:3000/api/v1/add -F "fileupload=@/tmp/file" -vvv

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/google/trillian"
	"github.com/projectrekor/rekor-server/logging"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

type API struct {
	conn      *grpc.ClientConn
	tLogID    int64
	logClient trillian.TrillianLogClient
}

func NewAPI() (*API, error) {
	tLogID := viper.GetInt64("trillian_log_server.tlog_id")
	logRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_log_server.address"),
		viper.GetInt("trillian_log_server.port"))
	conn, err := dial(context.Background(), logRpcServer)
	if err != nil {
		return nil, err
	}

	logClient := trillian.NewTrillianLogClient(conn)
	return &API{
		conn:      conn,
		tLogID:    tLogID,
		logClient: logClient,
	}, nil
}

func (api *API) ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong!")
}

func (api *API) getHandler(w http.ResponseWriter, r *http.Request) {
	tLogID := viper.GetInt64("trillian_log_server.tlog_id")
	file, header, err := r.FormFile("fileupload")

	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()

	// return that we have successfully uploaded our file!
	fmt.Fprintf(w, "Successfully Uploaded File\n")
	logging.Logger.Info("Received file : ", header.Filename)

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		writeError(w, err)
		return
	}

	server := serverInstance(api.logClient, api.tLogID)
	resp, err := server.getLeaf(byteLeaf, tLogID)
	if err != nil {
		writeError(w, err)
		return
	}
	logging.Logger.Infof("Server PUT Response: %s", resp.status)
	fmt.Fprintf(w, "Server PUT Response: %s\n", resp.status)
}

func (api *API) addHandler(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("fileupload")

	if err != nil {
		writeError(w, err)
		return
	}
	defer file.Close()

	// return that we have successfully uploaded our file!
	fmt.Fprintf(w, "Successfully Uploaded File\n")
	logging.Logger.Info("Received file : ", header.Filename)

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		writeError(w, err)
		return
	}

	server := serverInstance(api.logClient, api.tLogID)

	resp, err := server.addLeaf(byteLeaf, api.tLogID)
	if err != nil {
		writeError(w, err)
		return
	}
	logging.Logger.Infof("Server PUT Response: %s", resp.status)
	fmt.Fprintf(w, "Server PUT Response: %s", resp.status)
}

func New() (*chi.Mux, error) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	api, err := NewAPI()
	if err != nil {
		return nil, err
	}
	router.Post("/api/v1/add", api.addHandler)
	router.Post("/api/v1/get", api.getHandler)
	router.Get("/api/v1//ping", api.ping)
	return router, nil
}

func writeError(w http.ResponseWriter, err error) {
	logging.Logger.Error(err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "Server error: %v\n", err)
}
