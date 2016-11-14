package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

var config Config = Config{}

func init() {
	configFile := flag.String("config", "ddns.yml", "config file")
	flag.Parse()
	configData, err := ioutil.ReadFile(*configFile)
	if err != nil {
		log.Fatal("Error reading config file: ", *configFile)
	}
	yaml.Unmarshal(configData, &config)
}

func main() {
	log.Println(fmt.Sprintf("Performing Dynamic DNS every %d seconds...", config.Interval))

	client := getClient(config.Token)

	for {
		ip, err := getIpAddr(config.Source)
		if err != nil {
			log.Println("Error getting local IP address.", err.Error())
			time.Sleep(time.Second * time.Duration(config.Interval))
			continue
		}

		for _, record := range config.Records {
			for _, subdomain := range record.Subdomains {
				go handleUpdate(client, record.Domain, subdomain, ip)
			}
		}

		time.Sleep(time.Second * time.Duration(config.Interval))
	}
}

func handleUpdate(client *godo.Client, domain string, subdomain string, ip string) {
	id := 0

	records, _, err := client.Domains.Records(domain, &godo.ListOptions{})
	if err != nil {
		log.Println(fmt.Sprintf("Error getting records for %s from Digital Ocean API. %s", domain, err.Error()))
		return
	}

	for _, d := range records {
		if d.Name == subdomain {
			if d.Data == ip {
				// Record exists with the correct IP address
				return
			}
			id = d.ID
			break
		}
	}

	request := &godo.DomainRecordEditRequest{
		Type: "A",
		Name: subdomain,
		Data: ip,
	}

	// Check if we should update or create record
	if id == 0 {
		log.Println(fmt.Sprintf("Could not find record for %s.%s - creating one with IP %s.", subdomain, domain, ip))
		_, _, err := client.Domains.CreateRecord(domain, request)
		if err != nil {
			log.Println(fmt.Sprintf("Error creating record for %s.%s. %s", subdomain, domain, err.Error()))
			return
		}
	} else {
		_, _, err := client.Domains.EditRecord(domain, id, request)
		if err != nil {
			log.Println(fmt.Sprintf("Error updating record for %s.%s. %s", subdomain, domain, err.Error()))
			return
		}
		log.Println(fmt.Sprintf("Updated %s.%s with IP address %s.", subdomain, domain, ip))
	}
}

func getClient(token string) *godo.Client {
	tokenSource := &TokenSource{
		AccessToken: token,
	}
	oauthClient := oauth2.NewClient(oauth2.NoContext, tokenSource)
	client := godo.NewClient(oauthClient)
	return client
}

func getIpAddr(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(fmt.Sprintf("Received %d from %s", resp.StatusCode, url))
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

type TokenSource struct {
	AccessToken string
}

func (t *TokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}
	return token, nil
}

type Config struct {
	Token    string
	Source   string
	Interval int
	Records  []struct {
		Domain     string
		Subdomains []string
	}
}
