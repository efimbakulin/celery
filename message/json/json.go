/*
Copyright (c) 2014 Antonin Amand <antonin.amand@gmail.com>, All rights reserved.
See LICENSE file or http://www.opensource.org/licenses/BSD-3-Clause.
*/

package json

import (
	"encoding/json"
	"time"

	"github.com/gwik/gocelery/types"
)

type jsonMessage struct {
	Task    string                 `json:"task"`
	ID      string                 `json:"id"`
	Args    []interface{}          `json:"args"`
	Kwargs  map[string]interface{} `json:"kwargs"`
	Retries int                    `json:"retries"`
	Eta     string                 `json:"eta,omitempty"`
	Expires string                 `json:"expires,omitempty"`
}

func (jm *jsonMessage) ETA() time.Time {
	if jm.Eta == "" {
		return time.Now()
	}
	t, err := time.Parse(time.RFC3339Nano, jm.Eta)
	if err != nil {
		panic(err)
	}
	return t
}

func (jm *jsonMessage) ExpiresAt() time.Time {
	if jm.Expires == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, jm.Expires)
	if err != nil {
		panic(err)
	}
	return t
}

func decodeJSONMessage(p []byte) (*types.Message, error) {
	m := &jsonMessage{}
	err := json.Unmarshal(p, m)
	if err != nil {
		return nil, err
	}
	return &types.Message{
		Task:    m.Task,
		ID:      m.ID,
		Args:    m.Args,
		KwArgs:  m.Kwargs,
		Retries: m.Retries,
		ETA:     m.ETA(),
		Expires: m.ExpiresAt(),
	}, nil
}

func init() {
	types.RegisterMessageDecoder("application/json", decodeJSONMessage)
}