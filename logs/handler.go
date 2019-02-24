package logs

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/openfaas/faas-provider/httputils"
)

// Requestor submits queries the logging system.
// This will be passed to the log handler constructor.
type Requestor interface {
	// Filter allows the log handler to provide additional server side filtering of Messages.
	Filter(Request, Message) bool
	// Query submits a log request to the actual logging system.
	Query(context.Context, Request) (<-chan Message, error)
}

// NewLogHandlerFunc creates and http HandlerFunc from the supplied log Requestor.
func NewLogHandlerFunc(requestor Requestor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}

		cn, ok := w.(http.CloseNotifier)
		if !ok {
			log.Println("LogHandler: response is not a CloseNotifier, required for streaming response")
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Println("LogHandler: response is not a Flusher, required for streaming response")
			http.NotFound(w, r)
			return
		}

		logRequest, err := parseRequest(r)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			httputils.WriteError(w, http.StatusUnprocessableEntity, "could not parse the log request")
			return
		}

		ctx, cancelQuery := context.WithCancel(r.Context())
		defer cancelQuery()
		messages, err := requestor.Query(ctx, logRequest)
		if err != nil {
			// add smarter error handling here
			httputils.WriteError(w, http.StatusInternalServerError, "function log request failed")
			return
		}

		// Send the initial headers saying we're gonna stream the response.
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set(http.CanonicalHeaderKey("Content-Type"), "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		sent := 0
		jsonEncoder := json.NewEncoder(w)

		if logRequest.Limit > 0 {
			log.Printf("LogHandler: watch for and stream `%d` log messages\n", logRequest.Limit)
		}

		for messages != nil {
			select {
			case <-cn.CloseNotify():
				log.Println("LogHandler: client stopped listening")
				return
			case msg, ok := <-messages:
				if !ok {
					log.Println("LogHandler: end of log stream")
					messages = nil
					return
				}
				// maybe skip the filtering here and require the Query method to handle all of the filtering?
				if !requestor.Filter(logRequest, msg) {
					continue
				}
				// serialize and write the msg to the http ResponseWriter
				err := jsonEncoder.Encode(msg)
				if err != nil {
					// can't actually write the status header here so we should json serialize an error
					// and return that because we have already sent the content type and status code
					log.Printf("LogHandler: failed to serialize log message: '%s'\n", msg.String())
					// write json error message here ?
					jsonEncoder.Encode(Message{Text: "failed to serialize log message"})
					return
				}

				flusher.Flush()

				if logRequest.Limit > 0 {
					sent++
					if sent >= logRequest.Limit {
						log.Printf("LogHandler: reached message limit '%d'\n", logRequest.Limit)
						return
					}
				}
			}
		}

		return
	}
}

// parseRequest extracts the logRequest from the GET variables or from the POST body
func parseRequest(r *http.Request) (logRequest Request, err error) {
	switch r.Method {
	case http.MethodGet:
		query := r.URL.Query()
		logRequest.Name = getValue(query, "name")
		logRequest.Instance = getValue(query, "instance")
		limitStr := getValue(query, "limit")
		if limitStr != "" {
			logRequest.Limit, err = strconv.Atoi(limitStr)
			if err != nil {
				return logRequest, err
			}
		}
		// ignore error because it will default to false if we can't parse it
		logRequest.Follow, _ = strconv.ParseBool(getValue(query, "follow"))
		logRequest.Invert, _ = strconv.ParseBool(getValue(query, "invert"))

		sinceStr := getValue(query, "since")
		if sinceStr != "" {
			since, err := time.Parse(time.RFC3339, sinceStr)
			logRequest.Since = &since
			if err != nil {
				return logRequest, err
			}
		}

		// don't use getValue here so that we can detect if the value is nil or empty
		patterns := query["pattern"]
		if len(patterns) > 0 {
			logRequest.Pattern = &(patterns[len(patterns)-1])
		}

	case http.MethodPost:
		err = json.NewDecoder(r.Body).Decode(&logRequest)
	}

	return logRequest, err
}

// getValue returns the value for the given key. If the key has more than one value, it returns the
// last value. if the value does not exist, it returns the empty string.
func getValue(queryValues url.Values, name string) string {
	values := queryValues[name]
	if len(values) == 0 {
		return ""
	}

	return values[len(values)-1]
}
