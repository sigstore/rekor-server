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

// https://pace.dev/blog/2018/05/09/how-I-write-http-services-after-eight-years.html
// https://github.com/dhax/go-base/blob/master/api/api.go
// curl http://localhost:3000/add -F "fileupload=@/tmp/file" -vvv

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/google/trillian"
	"github.com/projectrekor/rekor-server/logging"
	"github.com/spf13/viper"
)

func ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong!")
}

func getHandler(w http.ResponseWriter, r *http.Request) {
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

	// fetch an GPRC connection
	ctx := r.Context()
	logRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_log_server.address"),
		viper.GetInt("trillian_log_server.port"))
	connection, err := dial(ctx, logRpcServer)
	if err != nil {
		writeError(w, err)
		return
	}

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		writeError(w, err)
		return
	}

	tLogClient := trillian.NewTrillianLogClient(connection)
	server := serverInstance(tLogClient, tLogID)

	resp, err := server.getLeaf(byteLeaf, tLogID)
	if err != nil {
		writeError(w, err)
		return
	}
	logging.Logger.Infof("Server PUT Response: %s", resp.status)
	fmt.Fprintf(w, "Server PUT Response: %s\n", resp.status)
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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

	// fetch an GPRC connection
	logRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_log_server.address"),
		viper.GetInt("trillian_log_server.port"))
	connection, err := dial(ctx, logRpcServer)
	if err != nil {
		writeError(w, err)
		return
	}

	leafFile, err := os.Open(header.Filename)
	defer leafFile.Close()
	if err != nil {
		writeError(w, err)
		return
	}

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		writeError(w, err)
		return
	}

	tLogClient := trillian.NewTrillianLogClient(connection)
	server := serverInstance(tLogClient, tLogID)

	resp, err := server.addLeaf(byteLeaf, tLogID)
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

	router.Post("/api/v1/add", addHandler)
	router.Post("/api/v1/get", getHandler)
	router.Get("/api/v1//ping", ping)
	return router, nil
}

func writeError(w http.ResponseWriter, err error) {
	logging.Logger.Error(err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "Server error: %v\n", err)
}
