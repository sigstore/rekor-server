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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/google/trillian"
	"github.com/projectrekor/rekor-server/logging"
	"github.com/spf13/viper"
	"google.golang.org/grpc/codes"
)

type API struct {
	tLogID    int64
	logClient trillian.TrillianLogClient
	mapClient trillian.TrillianMapClient
}

// type RespCode struct {
// 	Code codes.Code
// }

type RespCode struct {
	Code codes.Code
}

type RespCodeJson struct {
	Code string `json:"status_code"`
}

type FileRecieved struct {
	File string `json:"file_recieved"`
}

type ServerResponse struct {
	Status string `json:"server_status"`
}

func NewAPI() (*API, error) {
	logRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_log_server.address"),
		viper.GetInt("trillian_log_server.port"))
	ctx := context.Background()
	tConn, err := dial(ctx, logRpcServer)
	if err != nil {
		return nil, err
	}
	logAdminClient := trillian.NewTrillianAdminClient(tConn)
	logClient := trillian.NewTrillianLogClient(tConn)

	tLogID := viper.GetInt64("trillian_log_server.tlog_id")
	if tLogID == 0 {
		t, err := createAndInitTree(ctx, logAdminClient, logClient)
		if err != nil {
			return nil, err
		}
		tLogID = t.TreeId
	}

	mapRpcServer := fmt.Sprintf("%s:%d",
		viper.GetString("trillian_map_server.address"),
		viper.GetInt("trillian_map_server.port"))
	mConn, err := dial(ctx, mapRpcServer)
	if err != nil {
		return nil, err
	}
	mapAdminClient := trillian.NewTrillianAdminClient(mConn)
	mapClient := trillian.NewTrillianMapClient(mConn)
	tMapID := viper.GetInt64("trillian_map_server.tmap_id")
	if tMapID == 0 {
		t, err := createAndInitMap(ctx, mapAdminClient, mapClient)
		if err != nil {
			return nil, err
		}
		tMapID = t.TreeId
	}

	return &API{
		tLogID:    tLogID,
		logClient: logClient,
		mapClient: mapClient,
	}, nil
}

func (api *API) ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong!")
}

type getResponse struct {
	ServerResponse ServerResponse
	FileRecieved   FileRecieved
	Leaves         []*trillian.LogLeaf
}

func (api *API) getHandler(r *http.Request) (interface{}, error) {
	file, header, err := r.FormFile("fileupload")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	logging.Logger.Info("Received file: ", header.Filename)

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	server := serverInstance(api.logClient, api.tLogID)
	resp, err := server.getLeaf(byteLeaf, api.tLogID)
	if err != nil {
		return nil, err
	}
	logging.Logger.Infof("TLOG Response: %s", resp.status)

	logResults := resp.getLeafResult.GetLeaves()
	logging.Logger.Info("Get Entry Result: ", logResults)

	return getResponse{
		ServerResponse: ServerResponse{Status: resp.status},
		FileRecieved:   FileRecieved{File: header.Filename},
		Leaves:         logResults,
	}, nil
}

type getProofResponse struct {
	ServerResponse ServerResponse
	FileRecieved   FileRecieved
	Proof          *trillian.GetInclusionProofByHashResponse
}

func (api *API) getProofHandler(r *http.Request) (interface{}, error) {
	file, header, err := r.FormFile("fileupload")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	logging.Logger.Info("Received file : ", header.Filename)

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	server := serverInstance(api.logClient, api.tLogID)
	resp, err := server.getProof(byteLeaf, api.tLogID)
	if err != nil {
		return nil, err
	}

	logging.Logger.Infof("TLOG PUT Response: %s", resp.status)

	// Return TLOG Results as JSON
	logResults := resp.getProofResult

	logging.Logger.Info("Get Entry Result: ", logResults)
	return getProofResponse{
		ServerResponse: ServerResponse{Status: resp.status},
		FileRecieved:   FileRecieved{File: header.Filename},
		Proof:          logResults,
	}, nil

}

type addResponse struct {
	RespCodeJson   RespCodeJson
	ServerResponse ServerResponse
}

func (api *API) addHandler(r *http.Request) (interface{}, error) {
	file, header, err := r.FormFile("fileupload")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	logging.Logger.Info("Received file : ", header.Filename)

	byteLeaf, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	server := serverInstance(api.logClient, api.tLogID)

	resp, err := server.addLeaf(byteLeaf, api.tLogID)
	if err != nil {
		return nil, err
	}

	var codeRes RespCodeJson
	switch resp.code {
	case codes.OK:
		codeRes = RespCodeJson{Code: "OK"}
	case codes.AlreadyExists:
		codeRes = RespCodeJson{Code: "Data Already Exists!"}
	default:
		codeRes = RespCodeJson{Code: "Server Error!"}
	}

	logging.Logger.Infof("Server PUT Response: %s", resp.status)
	return addResponse{
		ServerResponse: ServerResponse{Status: resp.status},
		RespCodeJson:   codeRes,
	}, nil
}

type apiHandler func(r *http.Request) (interface{}, error)

func wrap(h apiHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respObj, err := h(r)
		if err != nil {
			writeError(w, err)
		}
		b, err := json.Marshal(respObj)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, string(b))
	}
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
	router.Post("/api/v1/add", wrap(api.addHandler))
	router.Post("/api/v1/get", wrap(api.getHandler))
	router.Post("/api/v1/getproof", wrap(api.getProofHandler))
	router.Get("/api/v1//ping", api.ping)
	return router, nil
}

func writeError(w http.ResponseWriter, err error) {
	logging.Logger.Error(err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "Server error: %v\n", err)
}
