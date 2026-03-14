package main

import (
"fmt"
"net/http"
"os"
"github.com/SheltonZhu/115driver/pkg/driver"
)

func main() {
// 直接从我们的 DB/Config 取 Cookie 可能比较麻烦，我们模拟一个最小化的请求
    // 假设云盘 ID 为 2 的 Cookie 我们已经在日志里看过了
    cookie := "UID=102071024_A1_1741924230; CID=d7950c4367c37e0fc393d63; SEID=945f277d4984252057d1bbaa89b4798a; KID=cb05852de7e1cd"
    pickCode := "dca6m9g2hp73ix5na" // 示例中的种子吧 pickcode

    client := driver.New(driver.UA(driver.UA115Disk))
    client.SetCookies(&http.Cookie{Name: "UID", Value: "102071024_A1_1741924230"}, &http.Cookie{Name: "CID", Value: "d7950c4367c37e0fc393d63"}, &http.Cookie{Name: "SEID", Value: "945f277d4984252057d1bbaa89b4798a"}, &http.Cookie{Name: "KID", Value: "cb05852de7e1cd"})

    info, err := client.DownloadWithUAByAndroidAPI(pickCode, driver.UA115Disk)
    if err != nil {
        fmt.Printf("Error generating link: %v\n", err)
        return
    }
    fmt.Printf("Generated URL: %s\n", info.Url.Url)

    req, _ := http.NewRequest("GET", info.Url.Url, nil)
    req.Header.Set("User-Agent", driver.UA115Disk)
    req.Header.Set("Referer", "https://115.com/")
    // 如果 Header 里有 Cookie，也加上
    for k, v := range info.Header {
        for _, val := range v {
            req.Header.Add(k, val)
        }
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        fmt.Printf("Error fetching: %v\n", err)
        return
    }
    defer resp.Body.Close()
    fmt.Printf("Status: %d\n", resp.StatusCode)
}
