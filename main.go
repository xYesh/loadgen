package main

import (
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/goware/urlx"
	"github.com/jessevdk/go-flags"
)

var ResourceLibrary = "loadgen"
var ResourceVersion = "dev"

type Logger interface {
	Printf(format string, v ...interface{})
	Error(format string, v ...interface{})
	Fatal(format string, v ...interface{})
}

type logger struct {
	verbose bool
}

func NewLogger(verbose bool) Logger {
	return &logger{verbose}
}

func (l *logger) Error(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
}

func (l *logger) Fatal(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
	os.Exit(1)
}

func (l *logger) Printf(format string, v ...interface{}) {
	if l.verbose {
		fmt.Printf(format, v...)
	}
}

// loadgen generates telemetry loads for performance testing It can generate
// traces (and eventually metrics and logs) It can send them to honeycomb or to
// a local agent, and it can generate OTLP or Honeycomb-formatted traces. It's
// highly configurable:
//
// - nservices is the number of communicating services to simulate; they will be
// divided into a triangular tree of services where each service will only call its siblings and the next rank.
// Services are named for spices.
//
// - depth is the average depth (nesting level) of a trace.
// - spancount is the average number of spans in a trace.
// If spancount is less than depth, the trace will be truncated at spancount.
// If spancount is greater than depth, some of the spans will have siblings.
//
// - spanwidth is the average number of fields in a span; this will vary by
// service but will be the same for all calls to a given service, and the names
// and types of all fields for an service will be consistent even across runs of loadgen (randomness is seeded by service name).
// Fields in a span will be randomly selected between the following types:
// #   - int (rectangular min/max)
// #   - int (gaussian mean/stddev)
// #   - int upcounter
// #   - int updowncounter (min/max)
// #   - float (rectangular min/max)
// #   - float (gaussian mean/stddev)
// #   - string (from list)
// #   - string (random min/max length)
// #   - bool
// In addition, every span will always have the following fields:
// #   - service name
// #   - trace id
// #   - span id
// #   - parent span id
// #   - duration_ms
// #   - start_time
// #   - end_time
// #   - process_id (the process id of the loadgen process)
//
// - avgDuration is the average duration of a trace's root span in milliseconds; individual
// spans will be randomly assigned durations that will fit within the root span's duration.
//
// - maxTime is the total amount of time to spend generating traces (0 means no limit)
// - tracesPerSecond is the number of root spans to generate per second
// - traceCount is the maximum number of traces to generate; as soon as TraceCount is reached, the process stops (0 means no limit)
// - rampup and rampdown are the number of seconds to spend ramping up and down to the desired TPS

// Functionally, the system works by spinning up a number of goroutines, each of which
// generates a stream of spans. The number of goroutines needed will equal tracesPerSecond * avgDuration.
// Rampup and rampdown are handled only by increasing or decreasing the number of goroutines.

// If a mix of different kinds of traces is desired, multiple loadgen processes can be run.

type Options struct {
	Host       string        `long:"host" description:"the url of the host to receive the metrics (or honeycomb, dogfood, localhost)" default:"honeycomb"`
	Insecure   bool          `long:"insecure" description:"use this for http connections"`
	Sender     string        `long:"sender" description:"type of sender (honeycomb, otlp, stdout, dummy)" default:"honeycomb"`
	Dataset    string        `long:"dataset" description:"if set, sends all traces to the given dataset; otherwise, sends them to the dataset named for the service"`
	APIKey     string        `long:"apikey" description:"the honeycomb API key"`
	NServices  int           `long:"nservices" description:"the number of services to simulate" default:"1"`
	Depth      int           `long:"depth" description:"the average depth of a trace" default:"3"`
	SpanCount  int           `long:"spancount" description:"the average number of spans in a trace" default:"3"`
	SpanWidth  int           `long:"spanwidth" description:"the average number of random fields in a span beyond the standard ones" default:"10"`
	TPS        int           `long:"tps" description:"the number of traces to generate per second" default:"1"`
	TraceCount int           `long:"tracecount" description:"the maximum number of traces to generate (0 means no limit)" default:"1"`
	Duration   time.Duration `long:"duration" description:"the duration of a trace" default:"1s"`
	MaxTime    time.Duration `long:"maxtime" description:"the maximum time to spend generating traces (0 means no limit)" default:"60s"`
	Ramp       time.Duration `long:"ramp" description:"seconds to spend ramping up or down to the desired TPS" default:"1s"`
	Verbose    bool          `long:"verbose" description:"set to print status and progress messages"`
}

// parses the host information and returns a cleaned-up version to make
// it easier to make sure that things are properly specified
// exits if it can't make sense of it
func parseHost(log Logger, host string, insecure bool) *url.URL {
	switch host {
	case "honeycomb":
		host = "https://api.honeycomb.io:443"
	case "dogfood":
		host = "https://api-dogfood.honeycomb.io:443"
	case "localhost":
		host = "http://localhost:8889"
	default:
	}

	// if the scheme is not specified, fall back to the value of the insecure flag
	defaultScheme := "https"
	if insecure {
		defaultScheme = "http"
	}
	u, err := urlx.ParseWithDefaultScheme(host, defaultScheme)
	if err != nil {
		log.Fatal("unable to parse host: %s\n", err)
	}
	port := u.Port()
	if port == "" {
		port = "4317" // default GRPC port
	}
	return u
}

func formatURLForGRPC(u *url.URL) (string, bool) {
	// it's insecure if it's not https
	return fmt.Sprintf("%s:%s", u.Hostname(), u.Port()), u.Scheme != "https"
}

// TraceCounter reads spans from src and writes them to dest, stopping
// when it has read maxcount spans or when it receives a value on stop.
// If maxcount is 0, it will run until it receives a value on stop.
// It returns true if it stopped because of a value on stop, false otherwise.
func TraceCounter(log Logger, src, dest chan *Span, maxcount int64, stop chan struct{}) bool {
	var count int64

	defer func() {
		log.Printf("span counter exiting after %d spans\n", count)
	}()

	for {
		select {
		case <-stop:
			return true
		case span := <-src:
			dest <- span
			if span.IsRootSpan() {
				count++
			}
			if maxcount > 0 && count >= maxcount {
				return false
			}
		}
	}
}

func main() {
	var args Options

	parser := flags.NewParser(&args, flags.Default)

	if _, err := parser.Parse(); err != nil {
		switch flagsErr := err.(type) {
		case *flags.Error:
			if flagsErr.Type == flags.ErrHelp {
				os.Exit(0)
			}
			os.Exit(1)
		default:
			os.Exit(1)
		}
	}

	log := NewLogger(args.Verbose)
	u := parseHost(log, args.Host, args.Insecure)

	log.Printf("host: %s, dataset: %s, apikey: %s\n\n", u.String(), args.Dataset, args.APIKey)

	var sender Sender
	switch args.Sender {
	case "dummy":
		sender = NewDummySender(log)
	case "stdout":
		sender = NewStdoutSender(log)
	case "honeycomb":
		var err error
		sender, err = NewHoneycombSender(log, args, u.String())
		if err != nil {
			log.Fatal("error configuring honeycomb sender: %s\n", err)
		}
	case "otlp":
		// ctx := context.Background()

		// var headers = map[string]string{
		// 	"x-honeycomb-team":    args.APIKey,
		// 	"x-honeycomb-dataset": args.Dataset,
		// }
		host, insecure := formatURLForGRPC(u)
		sender = NewOTelHoneySender(log, args.Dataset, args.APIKey, host, insecure)
	}

	// create a stop channel so we can shut down gracefully
	stop := make(chan struct{})
	// and a waitgroup so we can wait for everything to finish
	wg := &sync.WaitGroup{}

	// catch ctrl-c and close the stop channel
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)
	// wg.Add(1)
	go func() {
		<-sigch
		log.Printf("\nshutting down\n")
		close(stop)
		// wg.Done()
	}()

	// start the sender to receive spans and forward them appropriately
	dest := make(chan *Span, 1000)
	sender.Run(wg, dest, stop)

	// start the load generator to create spans and send them on the source chan
	src := make(chan *Span, 1000)
	var generator Generator = NewTraceGenerator(log, args)
	wg.Add(1)
	go generator.Generate(args, wg, src, stop)

	// start the span counter to keep track of how many spans we've sent
	// and stop the generator when we've reached the limit
	wg.Add(1)
	go func() {
		if !TraceCounter(log, src, dest, int64(args.TraceCount), stop) {
			close(stop)
		}
		wg.Done()
	}()

	// wait for things to finish
	wg.Wait()
}
