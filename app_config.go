package agollo

import (
	"os"
	"strconv"
	"time"
	"fmt"
	"net/url"
	"github.com/cihub/seelog"
	"errors"
	"net/http"
	"io/ioutil"
)

const appConfigFileName  ="app.properties"

var (
	refresh_interval = 5 *time.Minute //5m
	refresh_interval_key = "apollo.refreshInterval"  //

	long_poll_interval = 5 *time.Second //5s
	long_poll_connect_timeout  = 1 * time.Minute //1m

	connect_timeout  = 1 * time.Second //1s
	read_timeout     = 5 * time.Second //5s
	//for on error retry
	on_error_retry_interval = 1 * time.Second //1s
	//for typed config cache of parser result, e.g. integer, double, long, etc.
	max_config_cache_size    = 500             //500 cache key
	config_cache_expire_time = 1 * time.Minute //1 minute

	//max retries connect apollo
	max_retries=5

	//refresh ip list
	refresh_ip_list_interval=20 *time.Minute //20m

	//appconfig
	appConfig *AppConfig
)

type AppConfig struct {
	AppId string `json:"appId"`
	Cluster string `json:"cluster"`
	NamespaceName string `json:"namespaceName"`
	Ip string `json:"ip"`
}

func init() {
	//init common
	initCommon()

	//init config
	initConfig()
}

func initCommon()  {

	initRefreshInterval()
}

func initConfig() {
	var err error
	//init config file
	appConfig,err = loadJsonConfig(appConfigFileName)

	if err!=nil{
		panic(err)
	}

	go func(appConfig *AppConfig) {
		apolloConfig:=&ApolloConfig{}
		apolloConfig.AppId=appConfig.AppId
		apolloConfig.Cluster=appConfig.Cluster
		apolloConfig.NamespaceName=appConfig.NamespaceName

		updateApolloConfig(apolloConfig)
	}(appConfig)
}

//set timer for update ip list
//interval : 20m
func initServerIpList() {
	t2 := time.NewTimer(refresh_ip_list_interval)
	for {
		select {
		case <-t2.C:
			syncServerIpList()
			t2.Reset(refresh_ip_list_interval)
		}
	}
}

//sync ip list from server
//then
//1.update cache
//2.store in disk
func syncServerIpList() {
	client := &http.Client{
		Timeout:connect_timeout,
	}

	appConfig:=GetAppConfig()
	if appConfig==nil{
		panic("can not find apollo config!please confirm!")
	}
	url:=getServicesConfigUrl(appConfig)
	seelog.Debug("url:",url)

	retry:=0
	var responseBody []byte
	var err error
	var res *http.Response
	for{
		retry++

		if retry>max_retries{
			break
		}

		res,err=client.Get(url)

		if res==nil||err!=nil{
			seelog.Error("Connect Apollo Server Fail,Error:",err)
			continue
		}

		//not modified break
		switch res.StatusCode {
		case http.StatusOK:
			responseBody, err = ioutil.ReadAll(res.Body)
			if err!=nil{
				seelog.Error("Connect Apollo Server Fail,Error:",err)
				continue
			}
			return
		default:
			seelog.Error("Connect Apollo Server Fail,Error:",err)
			if res!=nil{
				seelog.Error("Connect Apollo Server Fail,StatusCode:",res.StatusCode)
			}
			// if error then sleep
			time.Sleep(on_error_retry_interval)
			continue
		}
	}

	seelog.Debug(responseBody)

	seelog.Error("Over Max Retry Still Error,Error:",err)
	if err==nil{
		err=errors.New("Over Max Retry Still Error!")
	}
}

func GetAppConfig()*AppConfig  {
	return appConfig
}

func initRefreshInterval() error {
	customizedRefreshInterval:=os.Getenv(refresh_interval_key)
	if isNotEmpty(customizedRefreshInterval){
		interval,err:=strconv.Atoi(customizedRefreshInterval)
		if isNotNil(err) {
			seelog.Errorf("Config for apollo.refreshInterval is invalid:%s",customizedRefreshInterval)
			return err
		}
		refresh_interval=time.Duration(interval)
	}
	return nil
}

func getConfigUrl(config *AppConfig) string{
	current:=GetCurrentApolloConfig()
	return fmt.Sprintf("http://%s/configs/%s/%s/%s?releaseKey=%s&ip=%s",
		config.Ip,
		url.QueryEscape(config.AppId),
		url.QueryEscape(config.Cluster),
		url.QueryEscape(config.NamespaceName),
		url.QueryEscape(current.ReleaseKey),
		getInternal())
}

func getNotifyUrl(notifications string,config *AppConfig) string{
	return fmt.Sprintf("http://%s/notifications/v2?appId=%s&cluster=%s&notifications=%s",
		config.Ip,
		url.QueryEscape(config.AppId),
		url.QueryEscape(config.Cluster),
		url.QueryEscape(notifications))
}

func getServicesConfigUrl(config *AppConfig) string{
	return fmt.Sprintf("http://%s/services/config?appId=%s&ip=%s",
		config.Ip,
		url.QueryEscape(config.AppId),
		getInternal())
}