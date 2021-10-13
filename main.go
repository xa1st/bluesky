package main

import (
	"container/list"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	json1 "encoding/json"

	simplejson "github.com/bitly/go-simplejson"
	"github.com/jordan-wright/email"
	"gopkg.in/ini.v1"
)

const (
	// 常量定义
	APIURL string = "https://jkssl.linlehui001.com/pmcs/masterController/ctrl.json" // API接口地址
	// 小区ID
	AREAID string = "7235161"
	// 获取消息间隔，单位为：秒
	DURITION int = 3600
)

var (
	// 协程管理
	wg sync.WaitGroup

	// 详情页请求地址
	detailUrl string = "http://182.92.161.166:8090/pmcs/htmController/ctrl.htm"

	// 邮件信息
	mapMail = map[string]string{"host": "", "user": "", "pwd": ""}

	// 存放所有的公告的管道
	chanNoticeUrls chan string

	// 用于监控协程
	chanTask chan string

	// 用于邮件队列
	queueMail *list.List = list.New()

	// 要抽取信息的正则
	reNotice string = `(?s)<div style="text-align:center; width:100%;" class="noticeTitle">.*?<div><b>(.*?)<\/b><\/div>.*?<span class="signTime">(.*?)<\/span>.*?<span class="signTime">&nbsp;<\/span>.*?<span class="signTime">(.*?)<\/span>.*?<\/div>.*?<div class="notice-content" style="width:100%;">(.*?)<\/div>`
)

// 错误处理
func HandleError(err error, msg string) {
	if err != nil {
		log.Fatal(err)
	}
}

// 构造post参数
func PostData(action string) string {
	json, err := simplejson.NewJson([]byte(`{}`))
	if err != nil {
		HandleError(err, "创建参数失败")
	}
	err = json.UnmarshalJSON([]byte(`{
		"head": {"action": "` + action + `", "resultCode": "0", "errorMsg": "OK!"},
		"body": {
			"data" : {"cellid":"` + AREAID + `", "pagenum":"1", "minid":"", "versionFlag":"1"},
			"datastatic": {"appVersion":"android-1.2.6", "cellId":"` + AREAID + `", "fromType":"0", "imei":"010045025970362", "ip":"10.1.2.46", "sysVersion":"android5.1.1", "tel":"", "type":"0", "userId":"", "versionInfo":"skyblue"}
		}
	}`))
	if err != nil {
		HandleError(err, "生成POST信息出错")
	}
	data, err := json.MarshalJSON()
	if err != nil {
		HandleError(err, "生成POST信息出错")
	}
	return string(data)
}

// 获取指定页面内容
func DownUrl(postdata string, methods string) ([]byte, error) {
	client := &http.Client{}
	request, err := http.NewRequest("POST", APIURL, strings.NewReader(postdata))
	if methods == "GET" {
		request, err = http.NewRequest("GET", detailUrl+"?"+postdata, nil)
	}
	if err != nil {
		HandleError(err, "访问错误")
	}
	// 添加头部协议
	request.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	request.Header.Add("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7,ko;q=0.6,zh-TW;q=0.5")
	request.Header.Add("Connection", "keep-alive")
	request.Header.Add("User-Agent", "Mozilla/5.0 (Linux; Android 5.0; SM-G900P Build/LRX21T) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/93.0.4577.82 Mobile Safari/537.36")
	// 提交数据
	response, err := client.Do(request)
	if err != nil {
		return []byte(""), err
	}
	body, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()
	if err != nil {
		return body, err
	}
	return body, err
}

// 发邮件给我
func SendToMail(to, subject, body string) {
	hp := strings.Split(mapMail["host"], ":")
	auth := smtp.PlainAuth("", mapMail["user"], mapMail["password"], hp[0])
	mail := email.NewEmail()
	mail.From = "物业小助手 <" + mapMail["user"] + ">"
	mail.To = []string{mapMail["user"]}
	mail.Subject = subject
	mail.Text = []byte(body)
	err := mail.SendWithTLS(mapMail["host"], auth, &tls.Config{ServerName: hp[0]})
	if err != nil {
		fmt.Println("邮件发送失败:", err)
	}
}

// 获取公告列表
func GetNoticeList() []int64 {
	var arrayNotice = make([]int64, 0)
	body, err := DownUrl(PostData("notice"), "POST")
	if err != nil {
		HandleError(err, "访问远程地址出错")
	}
	json, err := simplejson.NewJson(body)
	if err != nil {
		HandleError(err, "远程信息格式不正确")
	}
	datalists := json.Get("body").Get("list").MustArray()
	now := time.Now()
	for _, list := range datalists {
		if item, ok := list.(map[string]interface{}); ok {
			// 因为此处判定是1小时运行1次，所以只提取当前1小时内的消息,断言
			pTime, err := time.Parse("2006-01-02 15:04:05", item["addtime"].(string))
			if err != nil {
				continue
			}
			duration := now.Sub(pTime)
			if duration > time.Duration(DURITION*int(time.Second)) {
				continue
			}
			itemid, _ := item["id"].(json1.Number).Int64()
			arrayNotice = append(arrayNotice, itemid)
		}
	}
	return arrayNotice
}

// 爬公告内容页
func getNoticeDetail(id string) {
	url := fmt.Sprintf("action=noticedetailHTML&cellId=%s&id=%s", AREAID, id)
	body, err := DownUrl(url, "GET")
	if err != nil {
		HandleError(err, "获取公告内容失败")
	}
	noticeMap := GetNoticeInfo(string(body))
	// 这里发邮件
	noticeMap["mailto"] = mapMail["user"]
	// 记录一下ID
	noticeMap["id"] = id
	// 写入邮件队列
	queueMail.PushBack(noticeMap)
	chanTask <- id + "." + noticeMap["title"] + "[" + noticeMap["time"] + "]"
	wg.Done()
}

// 用正则提取出来想要的内容
func GetNoticeInfo(body string) map[string]string {
	re := regexp.MustCompile(reNotice)
	results := re.FindAllStringSubmatch(body, -1)
	noticeMap := make(map[string]string)
	for _, result := range results {
		noticeMap["title"] = result[1]
		noticeMap["time"] = result[2]
		noticeMap["author"] = result[3]
		noticeMap["content"] = FilterHtml(result[4])
	}
	return noticeMap
}

// 过滤HTML标签
func FilterHtml(body string) string {
	// 过滤所有的空格
	re, _ := regexp.Compile(`\<[\S\s]+?\>`)
	body = re.ReplaceAllString(body, "")

	//去除连续的换行符
	re, _ = regexp.Compile(`(&nbsp;)+`)
	body = re.ReplaceAllString(body, "")

	//去除连续的换行符
	re, _ = regexp.Compile(`\s{2,}`)
	body = re.ReplaceAllString(body, "")
	return body
}

// 测试邮箱配置
func checkMailConf() {
	var cfg *ini.File
	cfg, err := ini.Load("./config.ini")
	if err != nil {
		HandleError(err, "发生错误")
	}
	mapMail = map[string]string{
		"host": cfg.Section("smtp").Key("host").String(),
		"user": cfg.Section("smtp").Key("user").String(),
		"pwd":  cfg.Section("smtp").Key("pwd").String(),
	}
	if mapMail["host"] == "" || mapMail["user"] == "" || mapMail["pwd"] == "" {
		err := fmt.Errorf("邮箱SMTP配置错误，请检查后再继续")
		HandleError(err, "发生错误")
	}
}

// 任务统计协程
func checkOK(threadNum int) {
	var count int
	for {
		url := <-chanTask
		fmt.Printf("%s 完成了爬取任务\n", url)
		count++
		if count == threadNum {
			close(chanNoticeUrls)
			break
		}
	}
	wg.Done()
}

func main() {
	// 第一步就开始判定是否存在邮件配置，如果邮件发送失败，就没有抓取的必要了
	checkMailConf()
	// 获取公告的ID集合
	noticeList := GetNoticeList()
	if len(noticeList) < 1 {
		fmt.Println("当前没有更新的公告内容")
		os.Exit(0)
	}
	// 1. 初始化管道
	chanNoticeUrls = make(chan string, 1000)
	// 2. 开启和公告相同数量的线程
	chanTask = make(chan string, len(noticeList))
	// 3. 开始多线程爬
	for _, v := range noticeList {
		wg.Add(1)
		go getNoticeDetail(strconv.FormatInt(v, 10))
	}
	// 4. 任务统计协和，统计所有任务是否都完成，完成就关闭通道
	wg.Add(1)
	checkOK(len(noticeList))
	// 5. 更新最后的
	wg.Wait()
	// 7. 慢慢的发邮件
	i := 0
	defer fmt.Printf("共有%d条新邮件，已经全部发送完成\n", queueMail.Len())
	for p := queueMail.Front(); p != nil; p = p.Next() {
		i++
		fmt.Printf("共有%d条新邮件，正在发送第%d封...", queueMail.Len(), i)
		mail := p.Value.(map[string]string)
		// 标题为空则无法发送
		if mail["title"] == "" {
			fmt.Printf("发送失败,id:%s,抓取内容为空...\n", mail["id"])
			continue
		}
		SendToMail(mail["mailto"], mail["title"]+"["+mail["time"]+"]", mail["content"])
		time.Sleep(time.Second * 10) // 每10秒发一次邮件，防止达到运营商限制
	}
}
