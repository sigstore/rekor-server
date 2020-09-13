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
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
	logRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_log_server.address"),
		viper.GetInt("trillian_log_server.port"))
	file, header, err := r.FormFile("fileupload")

	if err != nil {
		logging.Logger.Errorf("Error in r.FormFile ", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "{'error': %s}", err)
		return
	}
	defer file.Close()

	// return that we have successfully uploaded our file!
	fmt.Fprintf(w, "Successfully Uploaded File\n")
	logging.Logger.Info("Received file : ", header.Filename)

	// fetch an GPRC connection
	connection, err := dial(logRpcServer)
	if err != nil {
		fmt.Printf("%+v\n", err)
	}

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}

	tLogClient := trillian.NewTrillianLogClient(connection)
	server := serverInstance(tLogClient, tLogID)

	resp := &Response{}

	resp, err = server.getLeaf(byteLeaf, tLogID)
	logging.Logger.Infof("Server PUT Response: %s", resp.status)
	fmt.Fprintf(w, "Server PUT Response: %s\n", resp.status)
}

func addHandler(w http.ResponseWriter, r *http.Request) {
	tLogID := viper.GetInt64("trillian_log_server.tlog_id")
	logRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_log_server.address"),
		viper.GetInt("trillian_log_server.port"))
	file, header, err := r.FormFile("fileupload")

	if err != nil {
		logging.Logger.Errorf("Error in r.FormFile ", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "{'error': %s}", err)
		return
	}
	defer file.Close()

	out, err := os.Create(header.Filename)
	if err != nil {
		logging.Logger.Errorf("Unable to create the file for writing. Check your write access privilege.", err)
		fmt.Fprint(w, "Unable to create the file for writing. Check your write access privilege.", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
	defer out.Close()

	_, err = io.Copy(out, file)
	if err != nil {
		logging.Logger.Errorf("Error copying file.", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// return that we have successfully uploaded our file!
	fmt.Fprintf(w, "Successfully Uploaded File\n")
	logging.Logger.Info("Received file : ", header.Filename)

	// fetch an GPRC connection
	connection, err := dial(logRpcServer)
	if err != nil {
		fmt.Printf("%+v\n", err)
	}

	leafFile, err := os.Open(header.Filename)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	byteLeaf, _ := ioutil.ReadAll(leafFile)
	defer leafFile.Close()

	tLogClient := trillian.NewTrillianLogClient(connection)
	server := serverInstance(tLogClient, tLogID)

	resp := &Response{}

	resp, err = server.addLeaf(byteLeaf, tLogID)
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
