package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/prometheus/alertmanager/template"
)

const (
	Red   = "#ff0000"
	Green = "#00ff00"
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
	zoomSend(data, m["channel-id"][0], m["token"][0])
	asJson(w, http.StatusOK, "success")
}

func zoomSend(data template.Data, channelID string, token string) {
	message := zoomMessage{}
	message.IsMarkdownSupport = false
	switch data.Status {
	case "firing":
		message.Content.Head.Style.Color = Red
	case "resolved":
		message.Content.Head.Style.Color = Green
	}
	message.Content.Head.Text = fmt.Sprintf("%s (%s)", data.CommonAnnotations["summary"], data.Status)
	message.Content.Head.SubHead.Text = fmt.Sprintf("%s/#/alerts?receiver=%s", data.ExternalURL, data.Receiver)

	var body []Body

	for _, alert := range data.Alerts {
		var alertBody Body
		alertBody.Type = "message"
		alertBody.Text = alert.Annotations["description"]
		body = append(body, alertBody)
	}
	message.Content.Body = body

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
		log.Printf("Error Do NewRequest %s: %v", zoomUrl, err)
		return
	}
	if response.StatusCode != 200 {
		log.Printf("Error Do NewRequest url:%s: token:%v body:%v status:%v", zoomUrl, token, string(bytesRepresentation[:]), response.Status)
	}
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
	listenAddress := ":8080"
	if os.Getenv("PORT") != "" {
		listenAddress = ":" + os.Getenv("PORT")
	}
	log.Printf("listening on: %v", listenAddress)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
