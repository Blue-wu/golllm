package main

import (
	"fmt"
	"sync"
)

func gethttp(string2 string) []string {
	return []string{string2 + "111", string2 + "222"}
}
func gethttpContent(string2 string) string {
	return string2
}
func main() {
	//切片，uuid值
	uuids := gethttp("123")
	dataSetChan := make(chan []string, len(uuids))
	//多go获取
	var wg sync.WaitGroup
	wg.Add(len(uuids))
	for _, uuid := range uuids {
		go func(uuid string) {
			defer wg.Done()
			//获取数据
			content := gethttpContent(uuid)
			//存到一个数据集channel中
			dataSetChan <- content

		}(uuid)
	}
	//带执行完成
	wg.Wait()

	//打印
	fmt.Println(<-dataSetChan)

	//关闭所以资源

	close(dataSetChan)

}
