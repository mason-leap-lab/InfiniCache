package main

import (
	"bytes"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/wangaoone/redeo"
	"github.com/wangaoone/redeo/resp"
	"github.com/wangaoone/s3gof3r"
	"github.com/wangaoone/LambdaObjectstore/lib/logger"
	"io"
	"net"
	"time"
)

type Chunk struct {
	id   int64
	body []byte
}

type DataEntry struct {
	op              int
	status          int
	reqId           string
	chunkId         int64
	durationAppend  time.Duration
	durationFlush   time.Duration
	duration        time.Duration
}

const OP_GET = 0
const OP_SET = 1

var (
	//lambdaConn, _ = net.Dial("tcp", "54.204.180.34:6379") // 10Gbps ec2 server Proxy0
	//lambdaConn, _ = net.Dial("tcp", "172.31.18.174:6379") // 10Gbps ec2 server Proxy1
	lambdaConn, _ = net.Dial("tcp", "54.211.243.58:6379") // 10Gbps ec2 server Proxy0
	srv           = redeo.NewServer(nil)
	myMap         = make(map[string]*Chunk)
	isFirst       = true
	log           = logger.NilLogger
)

func HandleRequest() {
	done := make(chan struct{})
	dataGatherer := make(chan *DataEntry, 10)
	dataDepository := make([]*DataEntry, 0, 100)

	if isFirst == true {
		isFirst = false
		go func() {
			log.Debug("conn is", lambdaConn.LocalAddr(), lambdaConn.RemoteAddr())
			// Define handlers
			srv.HandleFunc("get", func(w resp.ResponseWriter, c *resp.Command) {
				t := time.Now()
				log.Debug("in the get function")

				connId, _ := c.Arg(0).Int()
				reqId := c.Arg(1).String()
				log.Debug("reqId is", reqId)
				key := c.Arg(3).String()

				//val, err := myCache.Get(key)
				//if err == false {
				//	log.Debug("not found")
				//}
				chunk, found := myMap[key]
				if found == false {
					log.Debug("%s not found", key)
					dataGatherer <- &DataEntry{ OP_GET, 404, reqId, -1, 0, 0, time.Since(t) }
					return
				}
				log.Debug("%s found, len: %d", key, len(chunk.body))

				// construct lambda store response
				w.AppendInt(connId)
				w.AppendBulkString(reqId)
				w.AppendInt(chunk.id)
				t2 := time.Now()
				w.AppendBulk(chunk.body)
				d2 := time.Since(t2)
				log.Debug("appendBody time is ", d2)

				t3 := time.Now()
				if err := w.Flush(); err != nil {
					log.Error("Error on get::flush(key %s): %v", key, err)
					dataGatherer <- &DataEntry{ OP_GET, 500, reqId, chunk.id, d2, 0, time.Since(t) }
					return
				}
				d3 := time.Since(t3)
				log.Debug("flush time is ", d3)

				dt := time.Since(t)
				log.Debug("duration time is", dt)
				log.Debug("get complete, key: %s, client id:%d, chunk id:%d", key, connId, chunk.id)
				dataGatherer <- &DataEntry{ OP_GET, 200, reqId, chunk.id, d2, d3, dt }
			})

			srv.HandleFunc("set", func(w resp.ResponseWriter, c *resp.Command) {
				t := time.Now()
				log.Debug("in the set function")
				//if c.ArgN() != 3 {
				//	w.AppendError(redeo.WrongNumberOfArgs(c.Name))
				//	return
				//}

				connId, _ := c.Arg(0).Int()
				reqId := c.Arg(1).String()
				log.Debug("reqId is ", reqId)
				chunkId, _ := c.Arg(2).Int()
				key := c.Arg(3).String()
				val := c.Arg(4).Bytes()
				myMap[key] = &Chunk{ chunkId, val }

				// write Key, clientId, chunkId, body back to server
				w.AppendInt(connId)
				w.AppendBulkString(reqId)
				w.AppendInt(chunkId)
				w.AppendInt(1)
				if err := w.Flush(); err != nil {
					log.Error("Error on set::flush(key %s): %v", key, err)
					dataGatherer <- &DataEntry{ OP_SET, 500, reqId, chunkId, 0, 0, time.Since(t) }
					return
				}

				log.Debug("set complete, key:%s, val len: %d, client id: %d, chunk id: %d", key, len(val), connId, chunkId)
				dataGatherer <- &DataEntry{ OP_SET, 200, reqId, chunkId, 0, 0, time.Since(t) }
			})

			srv.HandleFunc("data", func(w resp.ResponseWriter, c *resp.Command) {
				log.Debug("in the data function")

				w.AppendInt(int64(len(dataDepository)))
				if err := w.Flush(); err != nil {
					log.Error("Error on data::flush: %v", err)
					return
				}
				log.Debug("data complete")
			})

			srv.Serve_client(lambdaConn)
		}()
	}

	// data gathering
	go func() {
		for {
			select {
			case <-done:
				return
			case entry := <-dataGatherer:
				dataDepository = append(dataDepository, entry)
			}
		}
	}()

	// timeout control
	select {
	case <-done:
		return
	case <-time.After(120 * time.Second):
		log.Debug("Lambda timeout, going to return function")
		return
	}
}

func remoteGet(bucket string, key string) []byte {
	log.Debug("get from remote storage")
	k, err := s3gof3r.EnvKeys()
	if err != nil {
		log.Debug("%v", err)
	}

	s3 := s3gof3r.New("", k)
	b := s3.Bucket(bucket)

	reader, _, err := b.GetReader(key, nil)
	if err != nil {
		log.Debug("%v", err)
	}
	obj := streamToByte(reader)
	return obj
}

func streamToByte(stream io.Reader) []byte {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(stream)
	if err != nil {
		log.Debug("%v", err)
	}
	return buf.Bytes()
}

func main() {
	lambda.Start(HandleRequest)
}
