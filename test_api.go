package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

func main() {
	go func() {
		// Run a simple test against the server
		time.Sleep(3 * time.Second)
		resp, err := http.Get("http://127.0.0.1:8080/api/task/horario_grande_good_220cbd/score-history")
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Println("Response:", string(body))
	}()
	time.Sleep(5 * time.Second)
}
