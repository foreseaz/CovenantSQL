/*
 * Copyright 2018 The ThunderDB Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rpc

import (
	"net"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	. "github.com/smartystreets/goconvey/convey"
	"gitlab.com/thunderdb/ThunderDB/conf"
	"gitlab.com/thunderdb/ThunderDB/crypto/kms"
	"gitlab.com/thunderdb/ThunderDB/proto"
	"gitlab.com/thunderdb/ThunderDB/route"
	"gitlab.com/thunderdb/ThunderDB/utils"
	"gitlab.com/thunderdb/ThunderDB/utils/log"
)

const (
	localAddr   = "127.0.0.1:4444"
	localAddr2  = "127.0.0.1:4445"
	concurrency = 10
	packetCount = 100
)

var (
	baseDir        = utils.GetProjectSrcDir()
	testWorkingDir = FJ(baseDir, "./test/")
	logDir         = FJ(testWorkingDir, "./log/")
)

var FJ = filepath.Join

func server(c C, localAddr string, wg *sync.WaitGroup, p *SessionPool, n int) error {
	// Accept a TCP connection
	listener, err := net.Listen("tcp", localAddr)
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		c.So(err, ShouldBeNil)

		// Setup server side of yamux
		log.Println("creating server session")
		session, err := yamux.Server(conn, nil)
		c.So(err, ShouldBeNil)

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(i int, c2 C) {
				// Accept a stream
				//c2.So(err, ShouldBeNil)
				// Stream implements net.Conn
				// Listen for a message
				//c2.So(string(buf1), ShouldEqual, "ping")
				defer wg.Done()
				log.Println("accepting stream")
				stream, err := session.Accept()
				if err == nil {
					buf1 := make([]byte, 4)
					for i := 0; i < n; i++ {
						stream.Read(buf1)
						c2.So(string(buf1), ShouldEqual, "ping")
					}
					log.Debugf("buf#%d read done", i)
				}
			}(i, c)
		}
	}()
	return err
}

func BenchmarkSessionPool_Get(b *testing.B) {
	Convey("session pool", b, func(c C) {
		log.SetLevel(log.DebugLevel)
		p := newSessionPool(func(nodeID proto.NodeID) (net.Conn, error) {
			log.Debugf("creating new connection to %s", nodeID)
			return net.Dial("tcp", string(nodeID))
		})

		wg := &sync.WaitGroup{}

		server(c, localAddr, wg, p, b.N)
		b.ResetTimer()
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(c2 C, n int) {
				// Open a new stream
				// Stream implements net.Conn
				defer wg.Done()
				stream, err := p.Get(proto.NodeID(localAddr))
				c2.So(err, ShouldBeNil)
				for i := 0; i < n; i++ {
					_, err = stream.Write([]byte("ping"))
				}
			}(c, b.N)
		}
		wg.Wait()
	})
}

func TestNewSessionPool(t *testing.T) {
	Convey("session pool", t, func(c C) {
		log.SetLevel(log.DebugLevel)
		p := newSessionPool(func(nodeID proto.NodeID) (net.Conn, error) {
			log.Debugf("creating new connection to %s", nodeID)
			return net.Dial("tcp", string(nodeID))
		})

		wg := &sync.WaitGroup{}

		server(c, localAddr, wg, p, packetCount)
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(c2 C, n int) {
				// Open a new stream
				// Stream implements net.Conn
				defer wg.Done()
				stream, err := p.Get(proto.NodeID(localAddr))
				if err != nil {
					log.Errorf("get session failed: %s", err)
					return
				}
				c2.So(err, ShouldBeNil)
				for i := 0; i < n; i++ {
					_, err = stream.Write([]byte("ping"))
				}
			}(c, packetCount)
		}

		wg.Wait()
		c.So(p.Len(), ShouldEqual, 1)

		wg2 := &sync.WaitGroup{}
		server(c, localAddr2, wg2, p, packetCount)
		conn, _ := net.Dial("tcp", localAddr2)
		exists := p.Set(proto.NodeID(localAddr2), conn)
		c.So(exists, ShouldBeFalse)
		exists = p.Set(proto.NodeID(localAddr2), conn)
		c.So(exists, ShouldBeTrue)
		c.So(p.Len(), ShouldEqual, 2)

		for i := 0; i < concurrency; i++ {
			wg2.Add(1)
			go func(c2 C, n int) {
				// Open a new stream
				// Stream implements net.Conn
				defer wg2.Done()
				stream, err := p.Get(proto.NodeID(localAddr2))
				if err != nil {
					log.Errorf("get session failed: %s", err)
					return
				}
				c2.So(err, ShouldBeNil)
				for i := 0; i < n; i++ {
					_, err = stream.Write([]byte("ping"))
				}
			}(c, packetCount)
		}

		wg2.Wait()
		c.So(p.Len(), ShouldEqual, 2)

		p.Remove(proto.NodeID(localAddr2))
		c.So(p.Len(), ShouldEqual, 1)

		p.Close()
		c.So(p.Len(), ShouldEqual, 0)

	})

	Convey("session pool get instance", t, func(c C) {
		So(GetSessionPoolInstance(), ShouldNotBeNil)
		So(GetSessionPoolInstance() == GetSessionPoolInstance(), ShouldBeTrue)
	})
}

func TestSessionPool_Leak(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	//utils.Build()
	leader, err := utils.RunCommandNB(
		FJ(baseDir, "./bin/thunderdbd"),
		[]string{"-config", FJ(testWorkingDir, "./pool/leader.yaml")},
		"leader", testWorkingDir, logDir, false,
	)

	defer func() {
		leader.Process.Signal(syscall.SIGKILL)
	}()

	log.Debugf("leader pid %d", leader.Process.Pid)
	time.Sleep(5 * time.Second)

	conf.GConf, err = conf.LoadConfig(FJ(testWorkingDir, "./pool/client.yaml"))
	if err != nil {
		t.Errorf("load config from %s failed: %s", FJ(testWorkingDir, "./pool/client.yaml"), err)
	}
	rootPath := conf.GConf.WorkingRoot
	pubKeyStorePath := filepath.Join(rootPath, conf.GConf.PubKeyStoreFile)
	privateKeyPath := filepath.Join(rootPath, conf.GConf.PrivateKeyFile)

	route.InitKMS(pubKeyStorePath)
	var masterKey []byte

	err = kms.InitLocalKeyPair(privateKeyPath, masterKey)
	if err != nil {
		t.Errorf("init local key pair failed: %s", err)
		return
	}

	leaderNodeID := kms.BP.NodeID

	nodePayload := proto.NewNode()
	nodePayload.InitNodeCryptoInfo(100 * time.Millisecond)
	nodePayload.Addr = "nodePayloadAddr"

	var reqType = "Ping"
	reqPing := &proto.PingReq{
		Node: *nodePayload,
	}
	respPing := new(proto.PingResp)
	caller := NewCaller()
	err = caller.CallNode(leaderNodeID, "DHT."+reqType, reqPing, respPing)
	log.Debugf("respPing %s: %##v", reqType, respPing)
	if err != nil {
		t.Error(err)
	}

	reqType = "FindNode"
	reqFN := &proto.FindNodeReq{
		NodeID: nodePayload.ID,
	}
	respFN := new(proto.FindNodeResp)
	err = caller.CallNode(leaderNodeID, "DHT."+reqType, reqFN, respFN)
	log.Debugf("respFN %s: %##v", reqType, respFN.Node)
	if err != nil || respFN.Node.Addr != "nodePayloadAddr" {
		t.Error(err)
	}

	pool := GetSessionPoolInstance()
	sess, _ := pool.getSessionFromPool(leaderNodeID)
	log.Debugf("session for %s, %#v", leaderNodeID, sess)
	//sess.Close()

	err = caller.CallNode(leaderNodeID, "DHT."+reqType, reqPing, respPing)
	log.Debugf("respPing %s: %##v", reqType, respPing)
	if err != nil {
		t.Error(err)
	}

}
