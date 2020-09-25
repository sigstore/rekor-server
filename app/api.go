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

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/projectrekor/rekor-server/logging"
	"github.com/spf13/viper"
)

type API struct {
	tLogID    int64
	logClient trillian.TrillianLogClient
	mapClient trillian.TrillianMapClient
	pubkey    *keyspb.PublicKey
}

type addResponse struct {
	Status RespStatusCode
}

type RespStatusCode struct {
	Code string `json:"status_code"`
}

type FileRecieved struct {
	File string `json:"file_recieved"`
}

type latestResponse struct {
	Proof *trillian.GetLatestSignedLogRootResponse
	Key   []byte
}

type getProofResponse struct {
	Status       RespStatusCode
	FileRecieved FileRecieved
	Proof        *trillian.GetInclusionProofByHashResponse
	Key          []byte
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

	t, err := logAdminClient.GetTree(ctx, &trillian.GetTreeRequest{
		TreeId: tLogID,
	})
	if err != nil {
		return nil, err
	}

	return &API{
		tLogID:    tLogID,
		logClient: logClient,
		mapClient: mapClient,
		pubkey:    t.PublicKey,
	}, nil
}

func (api *API) ping(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "pong!")
}

type getResponse struct {
	FileRecieved FileRecieved
	Leaves       []*trillian.LogLeaf
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

	return getResponse{
		FileRecieved: FileRecieved{File: header.Filename},
		Leaves:       logResults,
	}, nil
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

	proofResults := resp.getProofResult
	proofResultsJSON, err := json.Marshal(proofResults)

	logging.Logger.Info("Return Proof Result: ", string(proofResultsJSON))
	addResult := RespStatusCode{Code: getGprcCode(resp.status)}
	logging.Logger.Info((addResult))

	return getProofResponse{
		Status:       addResult,
		FileRecieved: FileRecieved{File: header.Filename},
		Proof:        proofResults,
		Key:          api.pubkey.Der,
	}, nil

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

	logging.Logger.Infof("Server PUT Response: %s", resp.status)

	addResult := RespStatusCode{Code: getGprcCode(resp.status)}

	return addResponse{
		Status: addResult,
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

func (api *API) latestHandler(r *http.Request) (interface{}, error) {
	lastSizeInt := int64(0)
	lastSize := r.URL.Query().Get("lastSize")
	logging.Logger.Info("Last Tree Recieved: ", lastSize)
	if lastSize != "" {
		var err error
		lastSizeInt, err = strconv.ParseInt(lastSize, 10, 64)
		if err != nil {
			return nil, err
		}
	}
	resp, err := api.logClient.GetLatestSignedLogRoot(r.Context(), &trillian.GetLatestSignedLogRootRequest{
		LogId:         api.tLogID,
		FirstTreeSize: lastSizeInt,
	})
	if err != nil {
		return nil, err
	}
	respJSON, err := json.Marshal(resp.SignedLogRoot)
	if err != nil {
		return nil, err
	}
	logging.Logger.Info("Return Latest Log Root:", string(respJSON))

	return latestResponse{
		Proof: resp,
		Key:   api.pubkey.Der,
	}, nil
}

type getLeafResponse struct {
	Leaf *trillian.GetLeavesByIndexResponse
	Key  []byte
}

func (api *API) getleafHandler(r *http.Request) (interface{}, error) {
	leafSizeInt := int64(0)
	leafIndex := r.URL.Query().Get("leafindex")

	// error check leaf index

	if leafIndex != "" {
		var err error
		leafSizeInt, err = strconv.ParseInt(leafIndex, 10, 64)
		if err != nil {
			return nil, err
		}
	}

	resp, err := api.logClient.GetLeavesByIndex(r.Context(), &trillian.GetLeavesByIndexRequest{
		LogId:     api.tLogID,
		LeafIndex: []int64{leafSizeInt},
	})

	if err != nil {
		return nil, err
	}
	logging.Logger.Info(resp)

	return getLeafResponse{
		Leaf: resp,
		Key:  api.pubkey.Der,
	}, nil
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
	router.Post("/api/v1/latest", wrap(api.latestHandler))
	router.Get("/api/v1/getleaf", wrap(api.getleafHandler))
	router.Get("/api/v1//ping", api.ping)
	return router, nil
}

func writeError(w http.ResponseWriter, err error) {
	logging.Logger.Error(err)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "Server error: %v\n", err)
}
