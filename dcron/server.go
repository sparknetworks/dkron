package dcron

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/carbocation/interpose"
	"github.com/gorilla/mux"
	"github.com/mitchellh/cli"
	"github.com/tylerb/graceful"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// ServerCommand run dcron server
type ServerCommand struct {
	Ui cli.Ui
}

func (s *ServerCommand) Help() string {
	helpText := `
Usage: dcron server [options]
	Provides debugging information for operators
Options:
  -format                  If provided, output is returned in the specified
                           format. Valid formats are 'json', and 'text' (default)
`
	return strings.TrimSpace(helpText)
}

func (s *ServerCommand) Run(args []string) int {
	var format string
	cmdFlags := flag.NewFlagSet("server", flag.ContinueOnError)
	cmdFlags.Usage = func() { s.Ui.Output(s.Help()) }
	cmdFlags.StringVar(&format, "format", "text", "output format")
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	go s.ServeHTTP()
	sched.Load()
	InitSerfAgent()
	return 0
}

func (s *ServerCommand) Synopsis() string {
	return "Run dcron server"
}

func (s *ServerCommand) ServeHTTP() {
	r := mux.NewRouter().StrictSlash(true)
	r.HandleFunc("/", IndexHandler)
	sub := r.PathPrefix("/jobs").Subrouter()
	sub.HandleFunc("/", JobCreateHandler).Methods("POST")
	sub.HandleFunc("/", JobsHandler).Methods("GET")

	middle := interpose.New()
	middle.UseHandler(r)

	srv := &graceful.Server{
		Timeout: 1 * time.Second,
		Server:  &http.Server{Addr: ":8080", Handler: middle},
	}

	log.Infoln("Running HTTP server on 8080")

	certFile := config.GetString("certFile")
	keyFile := config.GetString("keyFile")
	if certFile != "" && keyFile != "" {
		srv.ListenAndServeTLS(certFile, keyFile)
	} else {
		srv.ListenAndServe()
	}
	log.Debug("Exiting")
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	res := `{
  "status": 200,
  "name": ` + config.GetString("node_name") + `
}
`
	if _, err := fmt.Fprintln(w, res); err != nil {
		log.Fatal(err)
	}
}

func JobsHandler(w http.ResponseWriter, r *http.Request) {
	jobs, err := etcd.GetJobs()
	if err != nil {
		log.Error(err)
	}
	log.Debug(jobs)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(jobs); err != nil {
		log.Fatal(err)
	}
}

func JobCreateHandler(w http.ResponseWriter, r *http.Request) {
	var job Job
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 1048576))
	if err != nil {
		log.Fatal(err)
	}
	if err := r.Body.Close(); err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(body, &job); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity
		if err := json.NewEncoder(w).Encode(err); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Save the new job to etcd
	if err = etcd.SetJob(&job); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(422) // unprocessable entity
		if err := json.NewEncoder(w).Encode(err); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Schedule the new job
	sched.Reload()

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
	if _, err := fmt.Fprintf(w, `{"result": "ok"}`); err != nil {
		log.Fatal(err)
	}
}

func ExecutionsHandler(w http.ResponseWriter, r *http.Request) {
	executions, err := etcd.GetExecutions()
	if err != nil {
		log.Error(err)
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(executions); err != nil {
		panic(err)
	}
}
