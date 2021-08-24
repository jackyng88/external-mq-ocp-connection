package main

import (
	"os"
	"strings"
	"fmt"	
	"time"
	"encoding/hex"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

var qMgrName string
var qName string
var err error
var qMgr ibmmq.MQQueueManager
var rc int
var qObject ibmmq.MQObject

func main() {

	cno := ibmmq.NewMQCNO()
	sco := ibmmq.NewMQSCO()
	cd := ibmmq.NewMQCD()

	qMgrName = os.Getenv("QUEUE_MANAGER")
	qName = os.Getenv("QUEUE")
	cd.ChannelName = os.Getenv("CHANNEL")
	cd.ConnectionName = os.Getenv("CONNECTION_URL")
	sco.KeyRepository = os.Getenv("KEY_PATH")

	cd.SSLCipherSpec = "TLS_RSA_WITH_AES_128_CBC_SHA256"
	cd.SSLClientAuth = ibmmq.MQSCA_OPTIONAL

	cno.ClientConn = cd
	cno.Options = ibmmq.MQCNO_CLIENT_BINDING
	cno.SSLConfig = sco

	// Establish a connection
	qMgr, err = ibmmq.Connx(qMgrName, cno)
	// Connection successful
	if err == nil {
		fmt.Printf("Connection to %s succeeded.\n", qMgrName)
		rc = 0
	}

	if err != nil {
		fmt.Printf("Connection to %s failed.\n", qMgrName)
		fmt.Println(err)
		rc = int(err.(*ibmmq.MQReturn).MQCC)
	}

	// Call putMessage function.
	putMessage(qMgr)

	fmt.Println("Done.")
	qObject.Close(0)
	qMgr.Disc()
	os.Exit(rc)
	
}


func putMessage(qMgrObject ibmmq.MQQueueManager) {

	// Open the Queue
	mqod := ibmmq.NewMQOD()
	openOptions := ibmmq.MQOO_OUTPUT

	mqod.ObjectType = ibmmq.MQOT_Q
	mqod.ObjectName = qName

	qObject, err = qMgrObject.Open(mqod, openOptions)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Opened queue", qObject.Name)
	}

	// PUT a message into the queue
	putmqmd := ibmmq.NewMQMD()
	pmo := ibmmq.NewMQPMO()

	pmo.Options = ibmmq.MQPMO_NO_SYNCPOINT
	putmqmd.Format = ibmmq.MQFMT_STRING

	msgData := "Hello from Go at " + time.Now().Format(time.RFC3339)

	buffer := []byte(msgData)

	err = qObject.Put(putmqmd, pmo, buffer)

	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Put message into", strings.TrimSpace(qObject.Name))
		fmt.Println("MsgId: " + hex.EncodeToString(putmqmd.MsgId))
	}
}