package vkbot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Клиент против поддельного VK: формат запросов и разбор ответов.
func TestClient(t *testing.T) {
	var srv *httptest.Server
	var sent map[string]string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/upload" {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("upload: %v", err)
			}
			f, _, err := r.FormFile("photo")
			if err != nil {
				t.Errorf("upload без файла: %v", err)
			} else {
				data, _ := io.ReadAll(f)
				if string(data) != "PNGDATA" {
					t.Errorf("upload: %q", data)
				}
			}
			io.WriteString(w, `{"server":77,"photo":"[{\"x\":1}]","hash":"hh"}`)
			return
		}
		if r.URL.Path == "/lp" {
			if r.URL.Query().Get("ts") == "old" {
				io.WriteString(w, `{"failed":1,"ts":31}`)
				return
			}
			io.WriteString(w, `{"ts":"32","updates":[{"type":"message_new","object":{"message":{"id":9,"from_id":5,"peer_id":5,"text":"","geo":{"coordinates":{"latitude":44.64,"longitude":39.18}}}}}]}`)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("access_token") != "tok" || r.Form.Get("v") != apiVersion {
			t.Errorf("%s: нет токена или версии", r.URL.Path)
		}
		switch strings.TrimPrefix(r.URL.Path, "/method/") {
		case "groups.getLongPollServer":
			if r.Form.Get("group_id") != "243" {
				t.Errorf("group_id = %q", r.Form.Get("group_id"))
			}
			io.WriteString(w, `{"response":{"server":"`+srv.URL+`/lp","key":"k","ts":"30"}}`)
		case "messages.send":
			sent = map[string]string{}
			for k := range r.Form {
				sent[k] = r.Form.Get(k)
			}
			io.WriteString(w, `{"response":123}`)
		case "photos.getMessagesUploadServer":
			io.WriteString(w, `{"response":{"upload_url":"`+srv.URL+`/upload"}}`)
		case "photos.saveMessagesPhoto":
			if r.Form.Get("server") != "77" || r.Form.Get("hash") != "hh" {
				t.Errorf("saveMessagesPhoto: %v", r.Form)
			}
			io.WriteString(w, `{"response":[{"id":5,"owner_id":-243,"access_key":"ak"}]}`)
		case "users.get":
			io.WriteString(w, `{"response":[{"first_name":"Николай","last_name":"К"}]}`)
		case "bad":
			io.WriteString(w, `{"error":{"error_code":901,"error_msg":"Can't send messages"}}`)
		default:
			t.Errorf("неожиданный метод %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	c := &Client{Token: "tok", GroupID: 243, BaseURL: srv.URL + "/method/"}

	lp, err := c.LongPollServer(ctx)
	if err != nil || lp.Key != "k" || lp.TS != "30" {
		t.Fatalf("LongPollServer: %+v %v", lp, err)
	}
	res, err := c.Poll(ctx, LongPoll{Server: lp.Server, Key: "k", TS: "old"})
	if err != nil || res.Failed != 1 || res.TS != "31" {
		t.Fatalf("Poll failed: %+v %v", res, err)
	}
	res, err = c.Poll(ctx, lp)
	if err != nil || res.TS != "32" || len(res.Updates) != 1 {
		t.Fatalf("Poll: %+v %v", res, err)
	}
	if g := res.Updates[0].Object.Message.Geo; g == nil || g.Coordinates.Latitude != 44.64 {
		t.Fatalf("гео не разобрано: %+v", res.Updates[0])
	}

	lat, lng := 44.1, 39.2
	err = c.Send(ctx, Outgoing{PeerID: 5, Text: "привет", Attachments: []string{"photo1_2", "photo1_3"}, Keyboard: mainKeyboard(true), Lat: &lat, Lng: &lng})
	if err != nil {
		t.Fatal(err)
	}
	if sent["peer_id"] != "5" || sent["message"] != "привет" || sent["attachment"] != "photo1_2,photo1_3" || sent["random_id"] == "" || sent["lat"] != "44.100000" {
		t.Fatalf("messages.send: %v", sent)
	}
	var kb Keyboard
	if err := json.Unmarshal([]byte(sent["keyboard"]), &kb); err != nil || len(kb.Buttons) != 4 || kb.Buttons[1][0].Action.Type != "location" {
		t.Fatalf("клавиатура: %s %v", sent["keyboard"], err)
	}

	att, err := c.UploadPhoto(ctx, "a.png", strings.NewReader("PNGDATA"))
	if err != nil || att != "photo-243_5_ak" {
		t.Fatalf("UploadPhoto: %q %v", att, err)
	}
	first, last, err := c.UserName(ctx, 5)
	if err != nil || first != "Николай" || last != "К" {
		t.Fatalf("UserName: %q %q %v", first, last, err)
	}
	err = c.call(ctx, "bad", url.Values{}, nil)
	var ve *ErrVK
	if !errors.As(err, &ve) || ve.Code != 901 {
		t.Fatalf("ошибка VK: %v", err)
	}
}
