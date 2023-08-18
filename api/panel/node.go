package panel

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/InazumaV/V2bX/common/crypt"
	"github.com/goccy/go-json"
)

type CommonNodeRsp struct {
	Host       string     `json:"host"`
	ServerPort int        `json:"server_port"`
	ServerName string     `json:"server_name"`
	Routes     []Route    `json:"routes"`
	BaseConfig BaseConfig `json:"base_config"`
}

type Route struct {
	Id          int         `json:"id"`
	Match       interface{} `json:"match"`
	Action      string      `json:"action"`
	ActionValue string      `json:"action_value"`
}
type BaseConfig struct {
	PushInterval any `json:"push_interval"`
	PullInterval any `json:"pull_interval"`
}

type VMessNodeRsp struct {
	Tls             int             `json:"tls"`
	Network         string          `json:"network"`
	NetworkSettings json.RawMessage `json:"network_settings"`
}

type VLESSNodeRsp struct {
	Flow            string          `json:"flow"`
	Tls             int             `json:"tls"`
	TlsSettings     json.RawMessage `json:"tls_settings"`
	Network         string          `json:"network"`
	NetworkSettings json.RawMessage `json:"network_settings"`
}

type TlsSettings struct {
	ServerName string `json:"server_name"`
	ServerPort int    `json:"server_port"`
	ShortID    string `json:"short_id"`
}

type ShadowsocksNodeRsp struct {
	Cipher    string `json:"cipher"`
	ServerKey string `json:"server_key"`
}

type HysteriaNodeRsp struct {
	UpMbps   int    `json:"up_mbps"`
	DownMbps int    `json:"down_mbps"`
	Obfs     string `json:"obfs"`
}

type ExtraConfig struct {
	EnableVless   string         `json:"enable_vless"`
	Flow          string         `json:"flow"`
	EnableReality string         `json:"enable_reality"`
	RealityConfig *RealityConfig `json:"reality_config"`
}

type NodeInfo struct {
	Id              int
	Type            string
	Rules           Rules
	Host            string
	Port            int
	Network         string
	RawDNS          RawDNS
	ExtraConfig     ExtraConfig
	TlsSettings     TlsSettings
	NetworkSettings json.RawMessage
	Tls             bool
	ServerName      string
	UpMbps          int
	DownMbps        int
	ServerKey       string
	Cipher          string
	HyObfs          string
	PushInterval    time.Duration
	PullInterval    time.Duration
}

type RawDNS struct {
	DNSMap  map[string]map[string]interface{}
	DNSJson []byte
}

type Rules struct {
	Regexp   []string
	Protocol []string
}

type RealityConfig struct {
	Dest         string   `yaml:"Dest" json:"Dest"`
	Xver         string   `yaml:"Xver" json:"Xver"`
	ServerNames  []string `yaml:"ServerNames" json:"ServerNames"`
	PrivateKey   string   `yaml:"PrivateKey" json:"PrivateKey"`
	MinClientVer string   `yaml:"MinClientVer" json:"MinClientVer"`
	MaxClientVer string   `yaml:"MaxClientVer" json:"MaxClientVer"`
	MaxTimeDiff  string   `yaml:"MaxTimeDiff" json:"MaxTimeDiff"`
	ShortIds     []string `yaml:"ShortIds" json:"ShortIds"`
}

func (c *Client) GetNodeInfo() (node *NodeInfo, err error) {
	const path = "/api/v1/server/UniProxy/config"
	r, err := c.client.
		R().
		SetHeader("If-None-Match", c.nodeEtag).
		Get(path)
	if err = c.checkResponse(r, path, err); err != nil {
		return
	}
	if r.StatusCode() == 304 {
		return nil, nil
	}
	// parse common params
	node = &NodeInfo{
		Id:   c.NodeId,
		Type: c.NodeType,
		RawDNS: RawDNS{
			DNSMap:  make(map[string]map[string]interface{}),
			DNSJson: []byte(""),
		},
	}
	common := CommonNodeRsp{}
	err = json.Unmarshal(r.Body(), &common)
	if err != nil {
		return nil, fmt.Errorf("decode common params error: %s", err)
	}

	for i := range common.Routes {
		var matchs []string
		if _, ok := common.Routes[i].Match.(string); ok {
			matchs = strings.Split(common.Routes[i].Match.(string), ",")
		} else if _, ok = common.Routes[i].Match.([]string); ok {
			matchs = common.Routes[i].Match.([]string)
		} else {
			temp := common.Routes[i].Match.([]interface{})
			matchs = make([]string, len(temp))
			for i := range temp {
				matchs[i] = temp[i].(string)
			}
		}
		switch common.Routes[i].Action {
		case "block":
			for _, v := range matchs {
				if strings.HasPrefix(v, "protocol:") {
					// protocol
					node.Rules.Protocol = append(node.Rules.Protocol, strings.TrimPrefix(v, "protocol:"))
				} else {
					// domain
					node.Rules.Regexp = append(node.Rules.Regexp, strings.TrimPrefix(v, "regexp:"))
				}
			}
		case "dns":
			var domains []string
			for _, v := range matchs {
				domains = append(domains, v)
			}
			if matchs[0] != "main" {
				node.RawDNS.DNSMap[strconv.Itoa(i)] = map[string]interface{}{
					"address": common.Routes[i].ActionValue,
					"domains": domains,
				}
			} else {
				dns := []byte(strings.Join(matchs[1:], ""))
				node.RawDNS.DNSJson = dns
				break
			}
		}
	}
	node.ServerName = common.ServerName
	node.Host = common.Host
	node.Port = common.ServerPort
	node.PullInterval = intervalToTime(common.BaseConfig.PullInterval)
	node.PushInterval = intervalToTime(common.BaseConfig.PushInterval)
	// parse protocol params
	switch c.NodeType {
	case "vmess":
		rsp := VMessNodeRsp{}
		err = json.Unmarshal(r.Body(), &rsp)
		if err != nil {
			return nil, fmt.Errorf("decode v2ray params error: %s", err)
		}
		node.Network = rsp.Network
		node.NetworkSettings = rsp.NetworkSettings
		if rsp.Tls == 1 {
			node.Tls = true
		}
		err = json.Unmarshal(rsp.NetworkSettings, &node.ExtraConfig)
		if err != nil {
			return nil, fmt.Errorf("decode vless extra error: %s", err)
		}
		if node.ExtraConfig.EnableReality == "true" {
			if node.ExtraConfig.RealityConfig == nil {
				node.ExtraConfig.EnableReality = "false"
			} else {
				key := crypt.GenX25519Private([]byte(c.NodeType + c.Token))
				node.ExtraConfig.RealityConfig.PrivateKey = base64.RawURLEncoding.EncodeToString(key)
			}
		}
	case "vless":
		rsp := VLESSNodeRsp{}
		err = json.Unmarshal(r.Body(), &rsp)
		if err != nil {
			return nil, fmt.Errorf("decode v2ray params error: %s", err)
		}
		node.Network = rsp.Network
		node.NetworkSettings = rsp.NetworkSettings
		if rsp.Tls != 0 {
			node.Tls = true
		}
		if rsp.Tls == 2 {
			if err := json.Unmarshal(rsp.TlsSettings, &node.TlsSettings); err != nil {
				return nil, fmt.Errorf("decode vless extra error: %s", err)
			}
			key := crypt.GenX25519Private([]byte(c.NodeType + c.Token))
			node.ExtraConfig = ExtraConfig{
				Flow:          rsp.Flow,
				EnableReality: "true",
				RealityConfig: &RealityConfig{
					Dest: strconv.Itoa(node.TlsSettings.ServerPort),
					ServerNames: []string{
						node.TlsSettings.ServerName,
					},
					ShortIds: []string{
						node.TlsSettings.ShortID,
					},
					PrivateKey: base64.RawURLEncoding.EncodeToString(key),
				},
			}
		}
	case "shadowsocks":
		rsp := ShadowsocksNodeRsp{}
		err = json.Unmarshal(r.Body(), &rsp)
		if err != nil {
			return nil, fmt.Errorf("decode v2ray params error: %s", err)
		}
		node.ServerKey = rsp.ServerKey
		node.Cipher = rsp.Cipher
	case "trojan":
		node.Tls = true
	case "hysteria":
		rsp := HysteriaNodeRsp{}
		err = json.Unmarshal(r.Body(), &rsp)
		if err != nil {
			return nil, fmt.Errorf("decode v2ray params error: %s", err)
		}
		node.DownMbps = rsp.DownMbps
		node.UpMbps = rsp.UpMbps
		node.HyObfs = rsp.Obfs
	}
	c.nodeEtag = r.Header().Get("ETag")
	return
}

func intervalToTime(i interface{}) time.Duration {
	switch reflect.TypeOf(i).Kind() {
	case reflect.Int:
		return time.Duration(i.(int)) * time.Second
	case reflect.String:
		i, _ := strconv.Atoi(i.(string))
		return time.Duration(i) * time.Second
	case reflect.Float64:
		return time.Duration(i.(float64)) * time.Second
	default:
		return time.Duration(reflect.ValueOf(i).Int()) * time.Second
	}
}
