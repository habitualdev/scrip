package main

import (
	"flag"
	"fmt"
	"github.com/briandowns/spinner"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/proxy"
	"net"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func raw_connect(host string, ports []string) bool {
	for _, port := range ports {
		timeout := time.Second
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
		if err != nil {
			fmt.Println("Connecting error:", err)
			return false
		}
		if conn != nil {
			defer conn.Close()
			fmt.Println("Opened", net.JoinHostPort(host, port))
			return true
		}
	}
	return false
}

func main() {

	nCpus := runtime.NumCPU()

	quit := make(chan bool)
	pathBuffer := make(chan string, 1024)
	exit := make(chan bool)

	clear := flag.Bool("clear", false, "Use The clear internet")
	domain := flag.String("domain", "", "Domain to search")

	flag.Parse()

	u, _ := url.Parse(*domain)

	c := colly.NewCollector(colly.Async(true), colly.AllowedDomains(u.Host))
	c.Limit(&colly.LimitRule{DomainGlob: "*", Parallelism: nCpus})
	if !*clear {
		if raw_connect("127.0.0.1", []string{"9050"}) {
			println("Using proxy")
			prox, _ := proxy.RoundRobinProxySwitcher("socks5://127.0.0.1:9050")
			c.SetProxyFunc(prox)
		}
	}

	// Find and visit all links
	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		e.Request.Visit(e.Attr("href"))
	})

	c.OnRequest(func(r *colly.Request) {
	})

	c.OnResponse(func(r *colly.Response) {

		host := r.Request.URL.Host
		path := r.Request.URL.Path
		mime := r.Headers.Get("Content-Type")

		if mime == "application/octet-stream" {
			return
		}
		pathBuffer <- "{\"host\":\"" + host + "\",\"path\":\"" + path + "\",\"mime\":\"" + mime + "\",\"time\":\"" + time.Now().Format(time.RFC3339) + "\"}"
	})
	s := spinner.New(spinner.CharSets[32], 100*time.Millisecond)
	s.Color("red")
	s.Start()
	fmt.Println("starting web crawler")
	println(*domain)
	err := c.Visit(*domain)
	if err != nil {
		println(err.Error())
		return
	}
	go func() {
		addComma := false
		length := 1
		err := os.Remove("tree.json")
		if err != nil {
			println(err.Error())
		}

		filename := strings.Split(*domain, "//")[1]

		tree, _ := os.OpenFile(filename+".json", os.O_CREATE|os.O_RDWR, 0666)
		tree.Write([]byte("["))
		for {

			if addComma {
				tree.Write([]byte(","))
			} else if addComma == false {
				addComma = true
			}
			select {
			case path := <-pathBuffer:
				s.Suffix = " " + path
				write, err := tree.Write([]byte(path))
				if err != nil {
					return
				}
				length += write + 1
			case <-quit:
				goto Close
			}
		}
	Close:
		s.Stop()
		lastByte := make([]byte, 1)
		_, err = tree.ReadAt(lastByte, int64(length-1))
		if err != nil {
			println(err.Error())

		}
		if string(lastByte) == "," {
			_, err := tree.WriteAt([]byte("]"), int64(length-1))
			if err != nil {
				println(err.Error())
			}
			fmt.Println("Cleaning up json file...")
		} else {
			_, err := tree.Write([]byte("]"))
			if err != nil {
				println(err.Error())
			}
		}
		exit <- true
	}()

	ctrlC := make(chan os.Signal)
	signal.Notify(ctrlC, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctrlC
		quit <- true
		fmt.Println("\n Quitting early...")
		time.Sleep(100 * time.Millisecond)
		os.Exit(1)
	}()

	c.Wait()
	quit <- true
	<-exit

}
