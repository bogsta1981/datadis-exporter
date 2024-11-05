package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Supplies struct {
	PointType float64 `json:"pointType"`
}

type ContractRequestPayload struct {
	Cups        []string `json:"cups"`
	Distributor string   `json:"distributor"`
}

type Contract struct {
	ProvinciaCode    string  `json:"provinciaCode"`
	TarifaAccesoCode string  `json:"tarifaAccesoCode"`
	TipoAutoConsumo  *string `json:"tipoAutoConsumo"`
}

type ContractResponse struct {
	Response []Contract `json:"response"`
}

type ConsumptionRequestPayload struct {
	FechaInicial    string   `json:"fechaInicial"`
	FechaFinal      string   `json:"fechaFinal"`
	Cups            []string `json:"cups"`
	Distributor     string   `json:"distributor"`
	Fraccion        float64  `json:"fraccion"`
	HasAutoConsumo  bool     `json:"hasAutoConsumo"`
	ProvinceCode    string   `json:"provinceCode"`
	TarifaCode      string   `json:"tarifaCode"`
	TipoPuntoMedida float64  `json:"tipoPuntoMedida"`
	TipoAutoConsumo *string  `json:"tipoAutoConsumo"`
}

type Consumption struct {
	MeasureMagnitudeActive float64 `json:"measureMagnitudeActive"`
	Date                   string  `json:"date"`
	Hour                   string  `json:"hour"`
	Period                 string  `json:"period"`
}

type TimeCurveList struct {
	TimeCurveList []Consumption `json:"timeCurveList"`
}

type ConsumptionResponse struct {
	Response TimeCurveList `json:"response"`
}

type PowerRequestPayload struct {
	FechaInicial string   `json:"fechaInicial"`
	FechaFinal   string   `json:"fechaFinal"`
	Cups         []string `json:"cups"`
	Distributor  string   `json:"distributor"`
}

type Power struct {
	Periodo                 string  `json:"periodo"`
	MaximoPotenciaDemandada float64 `json:"maximoPotenciaDemandada"`
	Date                    string  `json:"fechaMaximo"`
	Hour                    string  `json:"hora"`
}

type PowerResponse struct {
	Response []Power `json:"response"`
}

type Config struct {
	DatadisUsername  string `json:"DatadisUsername"`
	DatadisPassword  string `json:"DatadisPassword"`
	Cups             string `json:"Cups"`
	DistributorCode  string `json:"DistributorCode"`
	Bucket           string `json:"Bucket"`
	InfluxDBHost     string `json:"InfluxDBHost"`
	InfluxDBApiToken string `json:"InfluxDBApiToken"`
	Org              string `json:"Org"`
}

const datadisLoginUrl = "https://datadis.es/nikola-auth/tokens/login"
const datadisSuppliesApiUrl = "https://datadis.es/api-private/api/get-supplies"
const datadisContractApiUrl = "https://datadis.es/api-private/supply-data/contractual-data"
const datadisConsumptionApiUrl = "https://datadis.es/api-private/supply-data/v2/time-curve-data/hours"
const datadisPowerApiUrl = "https://datadis.es/api-private/supply-data/max-power"

func main() {
	confFilePath := "datadis_exporter.json"
	confData, err := os.Open(confFilePath)
	if err != nil {
		log.Fatalln("Error reading config file: ", err)
	}
	defer confData.Close()
	var config Config
	err = json.NewDecoder(confData).Decode(&config)
	if err != nil {
		log.Fatalln("Error reading configuration: ", err)
	}
	if config.DatadisUsername == "" {
		log.Fatalln("DatadisUsername is required")
	}
	if config.DatadisPassword == "" {
		log.Fatalln("DatadisPassword is required")
	}
	if config.Cups == "" {
		log.Fatalln("Cups is required")
	}
	if config.DistributorCode == "" {
		log.Fatalln("DistributorCode is required")
	}
	if config.Bucket == "" {
		log.Fatalln("Bucket is required")
	}
	if config.InfluxDBHost == "" {
		log.Fatalln("InfluxDBHost is required")
	}
	if config.InfluxDBApiToken == "" {
		log.Fatalln("InfluxDBApiToken is required")
	}
	if config.Org == "" {
		log.Fatalln("Org is required")
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 60 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   60 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}

	data := "username=" + config.DatadisUsername + "&password=" + config.DatadisPassword + "&origin='WEB'"
	authReq, _ := http.NewRequest("POST", datadisLoginUrl, strings.NewReader(data))
	authReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authReq.Header.Set("User-Agent", "Mozilla/5.0")
	authResp, err := client.Do(authReq)
	if err != nil {
		log.Fatalln("Error trying to login: ", err)
	}
	defer authResp.Body.Close()
	authBody, err := io.ReadAll(authResp.Body)
	if err != nil {
		log.Fatalln("Error reading login data: ", err)
	}
	if authResp.StatusCode != http.StatusOK {
		log.Fatalln("Error trying to login:", string(authBody))
	}

	token := "Bearer " + string(authBody)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	var supplies []Supplies
	go func(supplies *[]Supplies) {
		defer wg.Done()
		suppliesReq, _ := http.NewRequest("GET", datadisSuppliesApiUrl, nil)
		suppliesReq.Header.Set("Accept", "application/json")
		suppliesReq.Header.Set("Accept-Encoding", "gzip")
		suppliesReq.Header.Set("Authorization", token)
		suppliesReq.Header.Set("User-Agent", "Mozilla/5.0")
		suppliesResp, err := client.Do(suppliesReq)
		if err != nil {
			log.Fatalln("Error requesting supplies data: ", err)
		}
		defer suppliesResp.Body.Close()
		suppliesJson, err := io.ReadAll(suppliesResp.Body)
		if err != nil {
			log.Fatalln("Error reading supplies data: ", err)
		}
		if suppliesResp.StatusCode != http.StatusOK {
			log.Fatalln("Error getting supplies data:", string(suppliesJson))
		}
		err = json.Unmarshal(suppliesJson, &supplies)
		if err != nil {
			log.Fatalln("Error unmarshalling supplies response data: ", err)
		}
	}(&supplies)

	wg.Add(1)
	var contracts ContractResponse
	go func(contracts *ContractResponse) {
		defer wg.Done()
		contractData := ContractRequestPayload{
			Cups:        []string{config.Cups},
			Distributor: config.DistributorCode,
		}
		contractDataJson, _ := json.Marshal(contractData)
		contractReq, _ := http.NewRequest("POST", datadisContractApiUrl, bytes.NewReader(contractDataJson))
		contractReq.Header.Set("Accept", "application/json")
		contractReq.Header.Set("Authorization", token)
		contractReq.Header.Set("Content-Type", "application/json")
		contractReq.Header.Set("User-Agent", "Mozilla/5.0")
		contractResp, err := client.Do(contractReq)
		if err != nil {
			log.Fatalln("Error requesting contract data: ", err)
		}
		defer contractResp.Body.Close()
		contractJson, err := io.ReadAll(contractResp.Body)
		if err != nil {
			log.Fatalln("Error reading contract data: ", err)
		}
		if contractResp.StatusCode != http.StatusOK {
			log.Fatalln("Error getting contract data:", string(contractJson))
		}
		err = json.Unmarshal(contractJson, &contracts)
		if err != nil {
			log.Fatalln("Error unmarshalling contracts response data: ", err, string(contractJson))
		}
	}(&contracts)

	wg.Wait()

	payload := bytes.Buffer{}
	wg.Add(1)
	go func(payload *bytes.Buffer) {
		defer wg.Done()

		lastMonth := time.Now().AddDate(0, -1, 0).Format("2006/01/02")
		today := time.Now().Format("2006/01/02")
		consumptionData := ConsumptionRequestPayload{
			FechaInicial:    lastMonth,
			FechaFinal:      today,
			Cups:            []string{config.Cups},
			Distributor:     config.DistributorCode,
			Fraccion:        0,
			HasAutoConsumo:  false,
			ProvinceCode:    contracts.Response[0].ProvinciaCode,
			TarifaCode:      contracts.Response[0].TarifaAccesoCode,
			TipoPuntoMedida: supplies[0].PointType,
			TipoAutoConsumo: contracts.Response[0].TipoAutoConsumo,
		}
		consumptionDataJson, _ := json.Marshal(consumptionData)
		consumptionReq, _ := http.NewRequest("POST", datadisConsumptionApiUrl, bytes.NewReader(consumptionDataJson))
		consumptionReq.Header.Set("Accept", "application/json")
		consumptionReq.Header.Set("Authorization", token)
		consumptionReq.Header.Set("Content-Type", "application/json")
		consumptionReq.Header.Set("User-Agent", "Mozilla/5.0")
		consumptionResp, err := client.Do(consumptionReq)
		if err != nil {
			log.Fatalln("Error requesting consumption data: ", err)
		}
		defer consumptionResp.Body.Close()
		consumptionJson, err := io.ReadAll(consumptionResp.Body)
		if err != nil {
			log.Fatalln("Error reading consumption data: ", err)
		}
		if consumptionResp.StatusCode != http.StatusOK {
			log.Fatalln("Error getting consumption data:", string(consumptionJson))
		}
		var consumption ConsumptionResponse
		err = json.Unmarshal(consumptionJson, &consumption)
		if err != nil {
			log.Fatalln("Error unmarshalling consumption response data: ", err)
		}

		for _, stat := range consumption.Response.TimeCurveList {
			if stat.Hour == "24:00" {
				stat.Hour = "00:00"
			}
			timestamp, err := time.Parse("2006/01/02 15:04", stat.Date+" "+stat.Hour)
			if err != nil {
				log.Fatalln("Error parsing timestamp: ", err)
			}
			switch stat.Period {
			case "PUNTA":
				stat.Period = "1"
			case "LLANO":
				stat.Period = "2"
			case "VALLE":
				stat.Period = "3"
			}
			influxLine := fmt.Sprintf("datadis_consumption,cups=%s,period=%s consumption=%.3f %v\n",
				config.Cups,
				stat.Period,
				stat.MeasureMagnitudeActive,
				timestamp.Unix(),
			)
			payload.WriteString(influxLine)

		}
	}(&payload)

	wg.Add(1)
	go func(payload *bytes.Buffer) {
		defer wg.Done()

		beginningOfYear := time.Date(time.Now().Year(), time.January, 1, 0, 0, 0, 0, time.UTC).Format("2006/01/02")
		endOfYear := time.Date(time.Now().Year(), time.December, 31, 0, 0, 0, 0, time.UTC).Format("2006/01/02")
		powerData := PowerRequestPayload{
			FechaInicial: beginningOfYear,
			FechaFinal:   endOfYear,
			Cups:         []string{config.Cups},
			Distributor:  config.DistributorCode,
		}
		powerDataJson, _ := json.Marshal(powerData)
		powerReq, _ := http.NewRequest("POST", datadisPowerApiUrl, bytes.NewReader(powerDataJson))
		powerReq.Header.Set("Accept", "application/json")
		powerReq.Header.Set("Authorization", token)
		powerReq.Header.Set("Content-Type", "application/json")
		powerReq.Header.Set("User-Agent", "Mozilla/5.0")
		powerResp, err := client.Do(powerReq)
		if err != nil {
			log.Fatalln("Error requesting power data: ", err)
		}
		defer powerResp.Body.Close()
		powerJson, err := io.ReadAll(powerResp.Body)
		if err != nil {
			log.Fatalln("Error reading power data: ", err)
		}
		if powerResp.StatusCode != http.StatusOK {
			log.Fatalln("Error getting power data:", string(powerJson))
		}
		var power PowerResponse
		err = json.Unmarshal(powerJson, &power)
		if err != nil {
			log.Fatalln("Error unmarshalling power response data: ", err)
		}

		for _, stat := range power.Response {
			if stat.Hour == "24:00" {
				stat.Hour = "00:00"
			}
			timestamp, err := time.Parse("2006/01/02 15:04", stat.Date+" "+stat.Hour)
			if err != nil {
				log.Fatalln("Error parsing timestamp:", err)
			}
			influxLine := fmt.Sprintf("datadis_power,cups=%s,period=%s max_power=%.3f %v\n",
				config.Cups,
				stat.Periodo,
				stat.MaximoPotenciaDemandada,
				timestamp.Unix(),
			)
			payload.WriteString(influxLine)

		}
	}(&payload)

	wg.Wait()

	if len(payload.Bytes()) == 0 {
		log.Fatalln("No data to send")
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(payload.Bytes())
	err = w.Close()
	if err != nil {
		log.Fatalln("Error compressing data: ", err)
	}
	url := fmt.Sprintf("https://%s/api/v2/write?precision=s&org=%s&bucket=%s", config.InfluxDBHost, config.Org, config.Bucket)
	post, _ := http.NewRequest("POST", url, &buf)
	post.Header.Set("Accept", "application/json")
	post.Header.Set("Authorization", "Token "+config.InfluxDBApiToken)
	post.Header.Set("Content-Encoding", "gzip")
	post.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := client.Do(post)
	if err != nil {
		log.Fatalln("Error sending data: ", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalln("Error reading data: ", err)
	}
	if resp.StatusCode != 204 {
		log.Fatal("Error sending data: ", string(body))
	}
}