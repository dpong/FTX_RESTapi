package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

const ENDPOINT = "https://ftx.com/api"

// Please do not send more than 10 requests per second. Sending requests more frequently will result in HTTP 429 errors.
type Client struct {
	key, secret string
	subaccount  string
	HTTPC       *http.Client
	Logger      *log.Logger
}

func New(key, secret, subaccount string, log *log.Logger) *Client {
	hc := &http.Client{
		Timeout: 10 * time.Second,
	}
	return &Client{
		key:        key,
		secret:     secret,
		subaccount: subaccount,
		HTTPC:      hc,
		Logger:     log,
	}
}

type Response struct {
	Success bool        `json:"success"`
	Result  interface{} `json:"result"`
}

func (p *Client) newRequest(method, spath string, body []byte, params *map[string]string) (*http.Request, error) {
	// avoid Pointer's butting
	u, _ := url.ParseRequestURI(ENDPOINT)
	u.Path = u.Path + spath
	if params != nil {
		q := u.Query()
		for k, v := range *params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	nonce := fmt.Sprintf("%d", time.Now().UTC().UnixNano()/1000000)
	var q string
	if u.RawQuery != "" {
		q = "?" + u.Query().Encode()
	}
	payload := nonce + method + u.Path + q
	if body != nil {
		payload += string(body)
	}
	signture := MakeHMAC(p.secret, payload)
	req, err := http.NewRequest(method, u.String(), strings.NewReader(string(body)))
	if err != nil {
		p.Logger.Println(err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("FTX-KEY", p.key)
	req.Header.Set("FTX-SIGN", signture)
	req.Header.Set("FTX-TS", nonce)
	if p.subaccount != "" {
		req.Header.Set("FTX-SUBACCOUNT", p.subaccount)
	}
	return req, nil
}

func MakeHMAC(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *Client) sendRequest(method, spath string, body []byte, params *map[string]string) (*http.Response, error) {
	req, err := c.newRequest(method, spath, body, params)
	if err != nil {
		c.Logger.Println(err)
		return nil, err
	}
	res, err := c.HTTPC.Do(req)
	if err != nil {
		c.Logger.Println(err)
		return nil, err
	}
	if res.StatusCode != 200 {
		//c.Logger.Printf("status: %s", res.Status)
		return nil, fmt.Errorf("faild to get data. status: %s", res.Status)
	}
	return res, nil
}

func decode(res *http.Response, out interface{}) error {
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	err := json.Unmarshal([]byte(body), &out)
	if err == nil {
		return nil
	}
	return err
}

func responseLog(res *http.Response) string {
	b, _ := httputil.DumpResponse(res, true)
	return string(b)
}
func requestLog(req *http.Request) string {
	b, _ := httputil.DumpRequest(req, true)
	return string(b)
}
