package main

import (
	"fmt"
	"github.com/neboduus/infinicache/proxy/client"
	"log"
	"strings"
)

func main() {
	var addrList = "10.4.0.100:6378"
	// initial object with random value

	// parse server address
	addrArr := strings.Split(addrList, ",")

	// initial new ecRedis client
	cli := client.NewClient(10, 2, 32, 3)

	// start dial and PUT/GET
	cli.Dial(addrArr)
	var data [][3]client.KVSetGroup

	var setStats []float32

	for k:=0; k<200; k++{
		d := cli.GenerateSetData()
		data = append(data, d)
		if _, stats, ok := cli.MkSet("foo", d); !ok {
			log.Fatal("Failed to mkSET %v", d)
		}else{
			setStats = append(setStats, stats)
			fmt.Println("Successfull mkSET %v", d)
		}
	}

	fmt.Println("Average mkSET time: %d", cli.Average(setStats))

	var getStats []float32
	getData := cli.GenerateRandomGet(data)
	for k:=0; k<len(getData); k++{
		d := getData[k]
		if res, stats, ok := cli.MkGet("foo", d); !ok {
			log.Fatal("Failed to mkGET %v", d)
		}else{
			getStats = append(getStats, stats)
			fmt.Println("Successfull mkGET %v", res)
		}
	}

	fmt.Println("Average mkGET time: %d", cli.Average(getStats))

}