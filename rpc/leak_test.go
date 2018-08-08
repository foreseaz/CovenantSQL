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
	"os"
	"syscall"
	"testing"
	"time"

	"gitlab.com/thunderdb/ThunderDB/conf"
	"gitlab.com/thunderdb/ThunderDB/crypto/kms"
	"gitlab.com/thunderdb/ThunderDB/proto"
	"gitlab.com/thunderdb/ThunderDB/route"
	"gitlab.com/thunderdb/ThunderDB/utils"
	"gitlab.com/thunderdb/ThunderDB/utils/log"
)

func TestSessionPool_SessionBroken(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	utils.Build()
	var err error
	conf.GConf, err = conf.LoadConfig(FJ(testWorkingDir, "./leak/client.yaml"))
	if err != nil {
		t.Errorf("load config from %s failed: %s", FJ(testWorkingDir, "./leak/client.yaml"), err)
	}
	log.Debugf("GConf: %##v", conf.GConf)
	rootPath := conf.GConf.WorkingRoot
	pubKeyStorePath := FJ(rootPath, conf.GConf.PubKeyStoreFile)
	privateKeyPath := FJ(rootPath, conf.GConf.PrivateKeyFile)
	os.Remove(pubKeyStorePath)
	os.Remove(FJ(testWorkingDir, "./leak/leader/dht.db"))
	os.Remove(FJ(testWorkingDir, "./leak/leader/dht.db-shm"))
	os.Remove(FJ(testWorkingDir, "./leak/leader/dht.db-wal"))
	os.Remove(FJ(testWorkingDir, "./leak/leader/kayak.db"))

	leader, err := utils.RunCommandNB(
		FJ(baseDir, "./bin/thunderdbd"),
		[]string{"-config", FJ(testWorkingDir, "./leak/leader.yaml")},
		"leak", testWorkingDir, logDir, false,
	)

	defer func() {
		leader.Process.Signal(syscall.SIGKILL)
	}()

	log.Debugf("leader pid %d", leader.Process.Pid)
	time.Sleep(5 * time.Second)

	route.InitKMS(pubKeyStorePath)
	var masterKey []byte

	err = kms.InitLocalKeyPair(privateKeyPath, masterKey)
	if err != nil {
		t.Errorf("init local key pair failed: %s", err)
		return
	}

	leaderNodeID := kms.BP.NodeID
	thisClient, _ := kms.GetNodeInfo(conf.GConf.ThisNodeID)

	var reqType string
	caller := NewCaller()

	reqType = "Ping"
	reqPing := &proto.PingReq{
		Node: *thisClient,
	}
	respPing := new(proto.PingResp)
	err = caller.CallNode(leaderNodeID, "DHT."+reqType, reqPing, respPing)
	log.Debugf("respPing %s: %##v", reqType, respPing)
	if err != nil {
		t.Error(err)
	}

	reqType = "Ping"
	reqPing = &proto.PingReq{
		Node: *thisClient,
	}
	respPing = new(proto.PingResp)
	err = caller.CallNode(leaderNodeID, "DHT."+reqType, reqPing, respPing)
	log.Debugf("respPing %s: %##v", reqType, respPing)
	if err != nil {
		t.Error(err)
	}

	reqType = "FindNode"
	reqFN := &proto.FindNodeReq{
		NodeID: thisClient.ID,
	}
	respFN := new(proto.FindNodeResp)
	err = caller.CallNode(leaderNodeID, "DHT."+reqType, reqFN, respFN)
	log.Debugf("respFN %s: %##v", reqType, respFN.Node)
	if err != nil {
		t.Error(err)
	}

	pool := GetSessionPoolInstance()
	sess, _ := pool.getSessionFromPool(leaderNodeID)
	log.Debugf("session for %s, %#v", leaderNodeID, sess)
	sess.Close()

	reqType = "FindNode"
	reqFN = &proto.FindNodeReq{
		NodeID: thisClient.ID,
	}
	respFN = new(proto.FindNodeResp)
	err = caller.CallNode(leaderNodeID, "DHT."+reqType, reqFN, respFN)
	log.Debugf("respFN %s: %##v", reqType, respFN.Node)
	if err != nil {
		t.Error(err)
	}
}