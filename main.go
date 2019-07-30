package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

var (
	tpl = template.Must(template.New("").Parse(`
<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Server-Side Events via nginx</title>
</head>
<body>
<h1>Server-Side Events</h1>
<p id="alert">Waiting for messages...</p>
<ul id="messages">
</ul>
<script>
function run() {
	var alert = document.getElementById('alert');
	var messages = document.getElementById('messages');
	
	if (typeof EventSource === 'undefined') {
		alert.innerHTML = "Your browser doesn't support Server-Side Events";
		return;
	}
	
	var evtSource = new EventSource('/events');
	console.log(evtSource.withCredentials);
	console.log(evtSource.readyState);
	console.log(evtSource.url);
	evtSource.onopen = function(e) {
		alert.innerHTML = 'Ready to receive events';
	}
	evtSource.onclose = function(e) {
		alert.innerHTML = 'Connection closed';
	}
	evtSource.onerror = function(e) {
		console.log(e);
		alert.innerHTML = 'Unable to receive events';
	}
	evtSource.addEventListener('time', function(e) {
		console.log(e);
		alert.innerHTML = 'Receiving';
	
		// Create element for new message
		var msg = document.createElement('li');
		if (e.event) {
			msg.innerHTML = e.event + ': ' + e.data;
		} else {
			msg.innerHTML = e.data;
		}
	
		// Append at the top of messages
		messages.insertBefore(msg, messages.firstChild);
	}, false);
}
run();
</script>
</body>
</html>
`))
)

func main() {
	rand.Seed(time.Now().UnixNano())

	if err := runMain(); err != nil {
		log.Fatal(err)
	}
}

func runMain() error {
	var (
		addr = flag.String("addr", ":3000", "HTTP address to bind to")
	)
	flag.Parse()

	r := mux.NewRouter()
	r.Handle("/", handleRoot(tpl))
	r.Handle("/events", handleEvents())

	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: r,
	}
	return httpSrv.ListenAndServe()
}

func handleRoot(tpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tpl.Execute(w, nil)
	}
}

func handleEvents() http.HandlerFunc {
	sendEvent := func(w http.ResponseWriter, event string, data string) {
		f := w.(http.Flusher)
		fmt.Fprintf(w, "id: %d\n", time.Now().UTC().UnixNano())
		if event != "" {
			fmt.Fprintf(w, "event: %s\n", event)
		}
		if data != "" {
			fmt.Fprintf(w, "data: %s\n", data)
		}
		fmt.Fprint(w, "\n")
		f.Flush()
	}

	return func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			http.Error(w,
				"unable to get http.Flusher interface; this is probably due "+
					"to nginx buffering the response",
				http.StatusInternalServerError)
			return
		}
		if want, have := "text/event-stream", r.Header.Get("Accept"); want != have {
			http.Error(w,
				fmt.Sprintf("Accept header: want %q, have %q; seems like the browser doesn't "+
					"support server-side events", want, have),
				http.StatusInternalServerError,
			)
			return
		}

		// Instruct nginx to NOT buffer the response
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		// Send a 2kb padding for old versions of IE
		fmt.Fprintf(w, ":%s\n", strings.Repeat(" ", 2049))
		fmt.Fprint(w, "retry: 2000\n\n")
		f.Flush()

		// Send heartbeats to ensure the connection stays up
		heartbeat := time.NewTicker(30 * time.Second)
		defer heartbeat.Stop()

		// This channel is closed when the browser closes the connection
		closeNotifier := w.(http.CloseNotifier).CloseNotify()

		for {
			select {
			case <-closeNotifier:
				f.Flush()
				return
			case <-heartbeat.C:
				sendEvent(w, "heartbeat", "{}")
			case t := <-time.After(time.Duration(rand.Int63n(10)) * time.Second):
				sendEvent(w, "time", t.Format(time.UnixDate))
			}
		}
	}
}
