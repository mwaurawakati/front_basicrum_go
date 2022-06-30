package it

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func SendBeacons() {

	valuesSlc := GetRealBeacons()

	cookieJar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:       100,
		IdleConnTimeout:    10 * time.Second,
		DisableCompression: true,
	}

	client := &http.Client{Transport: tr,
		Jar: cookieJar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}

	for i, v := range valuesSlc {
		httpPostForm(v, client, i)
		fmt.Println("Count:")
		fmt.Println(i)
	}
}

func httpPostForm(parm url.Values, client *http.Client, cnt int) {
	uaStr := parm.Get("user.agent")

	if len(uaStr) == 0 {
		uaStr = "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/39.0.2171.95 Safari/537.36"
	}

	countryCode := "DE"

	fmt.Println(strings.NewReader(parm.Encode()))
	req, _ := http.NewRequest("POST", "http://localhost:8087/beacon/catcher", strings.NewReader(parm.Encode()))
	req.Header.Add("User-Agent", uaStr)
	req.Header.Add("CF-IPCountry", countryCode)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)

	if err != nil {
		fmt.Println("Client err")
		fmt.Printf("%s", err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		fmt.Println("Body read err")
		fmt.Printf("%s", err)
	}

	fmt.Println(string(body))
}

func GetRealBeacons() []url.Values {
	files, _ := filepath.Glob("./data/**/*.json")

	valuesSlc := []url.Values{}

	for i, s := range files {
		if i > 20 {
			break
		}

		fmt.Println(i, s)

		fContent, err := ioutil.ReadFile(s)
		if err != nil {
			log.Fatalf("unable to read file: %v", err)
		}

		var fJson interface{}
		unmrErr := json.Unmarshal(fContent, &fJson)

		if unmrErr != nil {

			fmt.Println("Bad file")
			fmt.Println(unmrErr)
		}

		for _, value := range fJson.([]interface{}) {

			bMap := value.(map[string]interface{})

			var b interface{}
			unmrErr2 := json.Unmarshal([]byte(bMap["beacon_data"].(string)), &b)

			if unmrErr2 != nil {
				log.Printf("Bad beacon_data in file: %v", unmrErr2)
				continue
			}

			beaconData := b.(map[string]interface{})

			reqD := make(url.Values)

			for bK, bV := range beaconData {
				bKey := bK

				keyPart := ""

				if len(bKey) > 3 {
					keyPart = bKey[0:3]
				}

				if keyPart == "nt_" || bKey == "created_at" || bKey == "t_resp" || bKey == "t_done" || bKey == "t_page" || bKey == "t_other" {
					reqD.Set(bK, bV.(string))
					continue
				}

				reqD.Set(strings.ReplaceAll(bKey, "_", "."), bV.(string))
			}

			valuesSlc = append(valuesSlc, reqD)
		}
	}

	return valuesSlc
}