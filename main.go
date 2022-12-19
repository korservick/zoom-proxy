package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	Red            = "#ff0000"
	Orange         = "#ffa500"
	Green          = "#00ff00"
	MaxAlertCounts = 100
)

type responseJSON struct {
	Status  int
	Message string
}

type Body struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type zoomMessage struct {
	IsMarkdownSupport bool `json:"is_markdown_support"`
	Content           struct {
		Head struct {
			Style struct {
				Color string `json:"color"`
			} `json:"style"`
			Text    string `json:"text"`
			SubHead struct {
				Text string `json:"text"`
			} `json:"sub_head"`
		} `json:"head"`
		Body []Body `json:"body"`
	} `json:"content"`
}

var (
	alertsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "zoom_proxy_processed_alerts_total",
		Help: "The total number of processed alerts",
	})
	sendRequest = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "zoom_proxy_send_request_total",
		Help: "The total number of sending request by HTTP status code",
	}, []string{"code"},
	)
)

func asJson(w http.ResponseWriter, status int, message string) {
	data := responseJSON{
		Status:  status,
		Message: message,
	}
	dataBytes, _ := json.Marshal(data)
	dataJson := string(dataBytes[:])

	w.WriteHeader(status)
	_, err := fmt.Fprint(w, dataJson)
	if err != nil {
		log.Printf("ERROR asJson:%v", err)
	}
}

func webhook(w http.ResponseWriter, r *http.Request) {
	defer func() {
		err := r.Body.Close()
		if err != nil {
			log.Printf("ERROR r.Body.Close:%v", err)
		}
	}()
	data := template.Data{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		asJson(w, http.StatusBadRequest, err.Error())
		return
	}
	//log.Printf("Alerts: GroupLabels=%v, CommonLabels=%v", data.GroupLabels, data.CommonLabels)
	u, err := url.ParseRequestURI(r.RequestURI)
	if err != nil {
		asJson(w, http.StatusBadRequest, err.Error())
		return
	}
	m, _ := url.ParseQuery(u.RawQuery)
	//fmt.Println(m)
	zoomPrepare(data, m["channel-id"][0], m["token"][0])
	asJson(w, http.StatusOK, "success")
}

func zoomPrepare(data template.Data, channelID string, token string) {
	message := zoomMessage{}
	message.IsMarkdownSupport = false
	switch data.Status {
	case "firing":
		message.Content.Head.Style.Color = Red
		if data.CommonLabels["severity"] == "warning" {
			message.Content.Head.Style.Color = Orange
		}
	case "resolved":
		message.Content.Head.Style.Color = Green
	}
	message.Content.Head.Text = fmt.Sprintf("%s (%s) %s",
		data.CommonAnnotations["summary"], data.Status, data.CommonLabels["severity"])
	message.Content.Head.SubHead.Text = fmt.Sprintf("%s/#/alerts?receiver=%s", data.ExternalURL, data.Receiver)

	var body []Body
	var alertCounter int

	for _, alert := range data.Alerts {
		alertCounter += 1
		alertsProcessed.Inc()
		var alertBody Body
		alertBody.Type = "message"
		alertBody.Text = fmt.Sprintf("%s %s", alert.Annotations["description"], alert.Annotations["runbook"])
		if alert.Annotations["description"] == "" {
			log.Printf("Error alert %s description is empty", alert.Labels["alertname"])
		}
		if alertCounter > MaxAlertCounts {
			message.Content.Body = body
			zoomSend(message, channelID, token)
			body = nil
			alertCounter = 0
		}
		body = append(body, alertBody)
	}
	message.Content.Body = body
	zoomSend(message, channelID, token)
}

func zoomSend(message zoomMessage, channelID string, token string) {
	bytesRepresentation, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error Marshal %v: %v", message, err)
	}

	client := http.Client{}
	zoomUrl := fmt.Sprintf("https://inbots.zoom.us/incoming/hook/%s?format=full", channelID)
	request, err := http.NewRequest(http.MethodPost, zoomUrl, bytes.NewBuffer(bytesRepresentation))
	if err != nil {
		log.Printf("Error NewRequest %s: %v", zoomUrl, err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", token)
	response, err := client.Do(request)
	if err != nil {
		log.Printf("Error Do NewRequest url:%s :%v", zoomUrl, err)
		return
	}
	defer func() {
		err := response.Body.Close()
		if err != nil {
			log.Printf("Error close response body:%v", err)
		}
	}()
	if response.StatusCode != 200 {
		body, err := io.ReadAll(response.Body)
		// b, err := ioutil.ReadAll(resp.Body)  Go.1.15 and earlier
		if err != nil {
			log.Printf("Error read response body:%v", err)
		}

		log.Printf("Error Do NewRequest url:%s token:%v body:%v status code:%s status body:%s", zoomUrl, token, string(bytesRepresentation[:]), response.Status, string(body))
	}
	sendRequest.WithLabelValues(strconv.Itoa(response.StatusCode)).Inc()
}

func health(w http.ResponseWriter, _ *http.Request) {
	_, err := fmt.Fprint(w, "Ok!")
	if err != nil {
		log.Printf("ERROR asJson:%v", err)
	}
}

func main() {
	http.HandleFunc("/health", health)
	http.HandleFunc("/webhook", webhook)
	http.Handle("/metrics", promhttp.Handler())
	listenAddress := ":8080"
	if os.Getenv("PORT") != "" {
		listenAddress = ":" + os.Getenv("PORT")
	}
	log.Printf("listening on: %v", listenAddress)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
