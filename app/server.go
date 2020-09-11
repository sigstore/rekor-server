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
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/projectrekor/rekor-service/logging"
	"github.com/spf13/viper"
)

// Server provides an http.Server.
type Server struct {
	*http.Server
}

// NewServer creates and configures an APIServer serving all application routes.
func NewServer() (*Server, error) {
	api, err := New()
	if err != nil {
		return nil, err
	}

	var addr string
	port := viper.GetString("port")

	// allow port to be set as localhost:3000 in env during development to avoid "accept incoming network connection" request on restarts
	if strings.Contains(port, ":") {
		addr = port
	} else {
		addr = ":" + port
	}

	srv := http.Server{
		Addr:    addr,
		Handler: api,
	}

	return &Server{&srv}, nil
}

func (srv *Server) Start() {
	logging.Logger.Info("Starting server...")
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()
	logging.Logger.Infof("Listening on %s", srv.Addr)

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	sig := <-quit
	logging.Logger.Info("Shutting down server... Reason:", sig)

	if err := srv.Shutdown(context.Background()); err != nil {
		panic(err)
	}
	logging.Logger.Info("Server gracefully stopped")
}
