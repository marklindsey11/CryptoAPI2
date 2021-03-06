package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/libsv/go-bt"
)

type Client struct {
	client  *http.Client
	Url     string
	Timeout time.Duration
}

type RegisterRequest struct {
	Signature string `json:"signature"`
	PublicKey string `json:"publicKey"`
}

func New(url string, timeout time.Duration) *Client {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	return &Client{
		client:  client,
		Url:     url,
		Timeout: timeout,
	}
}

func (c *Client) Register(signature string, publicKey string, apiKey string) error {
	requestPayload := RegisterRequest{
		Signature: signature,
		PublicKey: publicKey,
	}

	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("failed to serialize: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s%s", c.Url, "/api/v1/pubkey"),
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	req = req.WithContext(ctx)

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("WARN: failed to close response body: %s", err.Error())
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response code: %v", resp.StatusCode)
	}

	return nil
}

type Payload struct {
	FeeTransaction  string `json:"feeTransaction"`
	DataTransaction string `json:"dataTransaction"`
}

func (c *Client) GetTransactionsTemplate(apiKey string, size int, feesRequired bool) (feeTx *bt.Tx, dataTx *bt.Tx, err error) {
	req, err := http.NewRequest(
		http.MethodGet,
		fmt.Sprintf("%s/api/v1/txTemplates?size=%d&feesRequired=%t", c.Url, size, feesRequired),
		nil,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	req = req.WithContext(ctx)

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to request: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("WARN: failed to close response body: %s", err.Error())
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected response code: %v", resp.StatusCode)
	}

	var payload Payload

	err = json.NewDecoder(resp.Body).Decode(&payload)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to deserialize request: %w", err)
	}

	feeTx, err = bt.NewTxFromString(payload.FeeTransaction)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to deserialize fee tx: %w", err)
	}

	dataTx, err = bt.NewTxFromString(payload.DataTransaction)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to deserialize data tx: %w", err)
	}

	return
}

func (c *Client) SubmitTransactions(apiKey string, feeTx *bt.Tx, dataTx *bt.Tx) error {
	request := &Payload{
		FeeTransaction:  feeTx.ToString(),
		DataTransaction: dataTx.ToString(),
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize request: %w", err)
	}

	if _, ok := os.LookupEnv("DEBUG"); ok {
		log.Printf("\nFUND: %s\nDATA: %s\n", request.FeeTransaction, request.DataTransaction)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s%s", c.Url, "/api/v1/submitTransactions"),
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	req = req.WithContext(ctx)

	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("WARN: failed to close response body: %s", err.Error())
		}
	}()

	if resp.StatusCode != http.StatusCreated {
		type errorResponse struct {
			Status int32  `json:"status"`
			Code   int32  `json:"code"`
			Err    string `json:"error"`
		}

		var errorResponsePayload errorResponse
		err = json.NewDecoder(resp.Body).Decode(&errorResponsePayload)
		if err != nil {
			return fmt.Errorf("failed to un-serialize response: %w", err)
		}

		return fmt.Errorf("failed to submit: %v", errorResponsePayload.Err)
	}

	return nil
}
