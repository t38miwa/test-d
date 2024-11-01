package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"hello-world/getAccessToken" 
)

// 各種構造体定義
type Movement struct {
	DriverID int `json:"driverId"`
	DbID     int `json:"dbId"`
	BranchID int `json:"branchId"`
}

type NippoApiResponse struct {
	Results struct {
		MovementDetail struct {
			DriverEigyousyoId   int    `json:"driverEigyousyoId"`
			RoadRunningDistance int    `json:"roadRunningDistance"` 
			LoadedDistance      int    `json:"loadedDistance"`
			EmptyDistance       int    `json:"emptyDistance"`         
			WorkStartTime       string `json:"workStartTime"`
			WorkEndTime         string `json:"workEndTime"`
			RoadRunningDuration int    `json:"roadRunningDuration"`
			WorkDuration        int    `json:"workDuration"`
			ShortRestDuration   int    `json:"shortRestDuration"`
			RestDuration        int    `json:"restDuration"`
		} `json:"movementDetail"`
		WorkDetailList []struct {
			StartDistance int    `json:"startDistance"`
			EndDistance   int    `json:"endDistance"`
			StartTime     string `json:"startTime"`
			EndTime       string `json:"endTime"`
			RecordName    string `json:"recordName"`
			LoadingKind   int    `json:"loadingKind"`
			RoadName      string `json:"roadName"`
		} `json:"workDetailList"`
	} `json:"results"`
}

type EigyousyoResponse struct {
	Results []struct {
		EigyousyoCode string `json:"eigyousyoCode"`
	} `json:"results"`
}

type ApiResponse struct {
	Results struct {
		MovementBasicInfo []Movement `json:"movementBasicInfo"`
	} `json:"results"`
}

// 営業所コードを取得する関数
func getEigyousyoCode(token string) ([]string, error) {
	url := "https://itpv3.transtron.fujitsu.com/openapi/v1/eigyousyo/info"
	method := "GET"

	tlsConfig := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+token)

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received status code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var eigyousyoResponse EigyousyoResponse
	err = json.Unmarshal(body, &eigyousyoResponse)
	if err != nil {
		return nil, err
	}

	var codes []string
	for _, result := range eigyousyoResponse.Results {
		codes = append(codes, result.EigyousyoCode)
	}

	log.Printf("Retrieved EigyousyoCodes: %v", codes)

	return codes, nil
}


// 運行情報を取得する関数
func getDriverAndDbIDs(token, codes string) ([]Movement, error) {
	url := fmt.Sprintf("https://itpv3.transtron.fujitsu.com/openapi/v1/dailyreport/movement?eigyousyoCode=%s", codes)
	method := "GET"

	tlsConfig := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+token)

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received status code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var apiResponse ApiResponse
	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		return nil, err
	}

	return apiResponse.Results.MovementBasicInfo, nil
}

// 日報データを取得する関数
func getNippoData(token string, driverID int, dbID int, wg *sync.WaitGroup, results chan<- struct{
	Movement Movement
	Data     *NippoApiResponse
}) {
	defer wg.Done()

	url := fmt.Sprintf("https://itpv3.transtron.fujitsu.com/openapi/v1/dailyreport/maintenancemodel?eigyousyoId=1&dbId=%d&driverId=%d&hasRoutePermission=false", dbID, driverID)

	tlsConfig := &tls.Config{
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 900 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		results <- struct{
			Movement Movement
			Data     *NippoApiResponse
		}{Movement: Movement{DriverID: driverID, DbID: dbID}, Data: nil}
		return
	}

	req.Header.Add("Authorization", "Bearer "+token)

	res, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		results <- struct{
			Movement Movement
			Data     *NippoApiResponse
		}{Movement: Movement{DriverID: driverID, DbID: dbID}, Data: nil}
		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		fmt.Printf("Received status code %d for driverId %d and dbId %d\n", res.StatusCode, driverID, dbID)
		results <- struct{
			Movement Movement
			Data     *NippoApiResponse
		}{Movement: Movement{DriverID: driverID, DbID: dbID}, Data: nil}
		return
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		results <- struct{
			Movement Movement
			Data     *NippoApiResponse
		}{Movement: Movement{DriverID: driverID, DbID: dbID}, Data: nil}
		return
	}

	var nippoApiResponse NippoApiResponse
	err = json.Unmarshal(body, &nippoApiResponse)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		results <- struct{
			Movement Movement
			Data     *NippoApiResponse
		}{Movement: Movement{DriverID: driverID, DbID: dbID}, Data: nil}
		return
	}

	results <- struct{
		Movement Movement
		Data     *NippoApiResponse
	}{Movement: Movement{DriverID: driverID, DbID: dbID}, Data: &nippoApiResponse}
}

// データを保存する関数（DynamoDBへの保存）
func saveToDynamoDB(client *dynamodb.Client, DriverEigyousyoId int, driverID int, dbID int, logData map[string]interface{}) error {
	tableName := os.Getenv("TABLE_NAME")

	item := map[string]types.AttributeValue{
		"DriverEigyousyoId": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", DriverEigyousyoId)},
		"DriverID":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", driverID)},
		"DbID":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", dbID)},
		"LogData":    &types.AttributeValueMemberS{Value: fmt.Sprintf("%v", logData)},
		"Timestamp":  &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
	}

	_, err := client.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(tableName),
		Item:      item,
	})

	return err
}

func lambdaHandler() {
    // AWS SDK の設定をロード
    cfg, err := config.LoadDefaultConfig(context.TODO())
    if err != nil {
        log.Fatalf("failed to load AWS config, %v", err)
    }

    // DynamoDB クライアントを作成
    dynamoClient := dynamodb.NewFromConfig(cfg)

    // アクセストークン取得
    token, err := getAccessToken.GetAccessToken()
    if err != nil {
        log.Printf("Failed to get access token: %v", err)
        return
    }

    // 営業所コードを取得
    codes, err := getEigyousyoCode(token)
    if err != nil {
        log.Printf("Failed to get eigyousyo code: %v", err)
        return
    }

    var wg sync.WaitGroup
    results := make(chan struct {
        Movement Movement
        Data     *NippoApiResponse
    }, len(codes)*10) // 適切なバッファサイズに設定（推定）

    // 各営業所コードごとに運行情報を取得
    for _, code := range codes {
        // 運行情報取得
        movements, err := getDriverAndDbIDs(token, code)
        if err != nil {
            log.Printf("Failed to get driver and db IDs for eigyousyoCode %s: %v", code, err)
            continue
        }

        // 各ドライバーの情報を取得
        for _, movement := range movements {
            wg.Add(1)
            go getNippoData(token, movement.DriverID, movement.DbID, &wg, results)
        }
    }

    // 結果を処理するためのゴルーチンを開始
    go func() {
        wg.Wait()
        close(results) // 全ての処理が完了したらチャンネルを閉じる
    }()

    // results チャンネルからデータを受け取って処理
    for result := range results {
        nippoData := result.Data
        movement := result.Movement

        // nippoData が nil かチェック
        if nippoData == nil {
            log.Printf("nippoData is nil for DriverID %d, DbID %d", movement.DriverID, movement.DbID)
            continue
        }

        // ログデータ作成
        logData := map[string]interface{}{
            "DriverID":            movement.DriverID,
            "DbID":                movement.DbID,
            "RoadRunningDistance": nippoData.Results.MovementDetail.RoadRunningDistance,
            "LoadedDistance":      nippoData.Results.MovementDetail.LoadedDistance,
            "EmptyDistance":       nippoData.Results.MovementDetail.EmptyDistance,
            "WorkStartTime":       nippoData.Results.MovementDetail.WorkStartTime,
            "WorkEndTime":         nippoData.Results.MovementDetail.WorkEndTime,
            "RoadRunningDuration": nippoData.Results.MovementDetail.RoadRunningDuration,
            "WorkDuration":        nippoData.Results.MovementDetail.WorkDuration,
        }

		log.Printf("ログデータ作成完了: %v", logData)

        // DynamoDB に保存
        err := saveToDynamoDB(dynamoClient, nippoData.Results.MovementDetail.DriverEigyousyoId, movement.DriverID, movement.DbID, logData)
        if err != nil {
            log.Printf("Failed to save to DynamoDB for DriverID %d, DbID %d: %v", movement.DriverID, movement.DbID, err)
        } else {
            log.Printf("Successfully saved to DynamoDB for DriverID %d, DbID %d", movement.DriverID, movement.DbID)
        }
    }
}

func main() {
    lambda.Start(lambdaHandler)
}

