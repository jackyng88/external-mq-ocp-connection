package main

import (
	"os"
	"strings"
	"fmt"	
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
	getMessage(qMgr)

	fmt.Println("Done.")
	qObject.Close(0)
	qMgr.Disc()
	os.Exit(rc)
	
}

func getMessage(qMgrObject ibmmq.MQQueueManager) {

	var msgId string

	// Open the Queue
	mqod := ibmmq.NewMQOD()

	// Instruction on how to use the Queue
	openOptions := ibmmq.MQOO_INPUT_EXCLUSIVE

	mqod.ObjectType = ibmmq.MQOT_Q
	mqod.ObjectName = qName

	qObject, err = qMgrObject.Open(mqod, openOptions)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Opened queue", qObject.Name)
	}

	// GET messages from the queue
	msgAvail := true
	for msgAvail == true && err == nil {

		var datalen int

		getmqmd := ibmmq.NewMQMD()
		gmo := ibmmq.NewMQGMO()
		gmo.Options = ibmmq.MQGMO_NO_SYNCPOINT

		// Set options to wait for a maximum of 3 seconds for any new message to arrive
		gmo.Options |= ibmmq.MQGMO_WAIT
		gmo.WaitInterval = 3 * 1000 // The WaitInterval is in milliseconds

		// If there is a MsgId on the command line decode it into bytes and
		// set the options for matching it during the Get processing
		if msgId != "" {
			fmt.Println("Setting Match Option for MsgId")
			gmo.MatchOptions = ibmmq.MQMO_MATCH_MSG_ID
			getmqmd.MsgId, _ = hex.DecodeString(msgId)
			msgAvail = false
		}

		// There are now two forms of the Get verb.
		// The original Get() takes
		// a buffer and returns the length of the message. The user can then
		// use a slice operation to extract just the relevant data.
		//
		// The new GetSlice() returns the message data pre-sliced as an extra
		// return value.
		//
		// This boolean just determines which Get variation is demonstrated in the sample
		useGetSlice := true
		if useGetSlice {
			// Create a buffer for the message data. This one is large enough
			// for the messages put by the amqsput sample. Note that in this case
			// the make() operation is just allocating space - len(buffer)==0 initially.
			buffer := make([]byte, 0, 1024)

			// Now we can try to get the message. This operation returns
			// a buffer that can be used directly.
			buffer, datalen, err = qObject.GetSlice(getmqmd, gmo, buffer)

			if err != nil {
				msgAvail = false
				fmt.Println(err)
				mqret := err.(*ibmmq.MQReturn)
				if mqret.MQRC == ibmmq.MQRC_NO_MSG_AVAILABLE {
					// If there's no message available, then I won't treat that as a real error as
					// it's an expected situation
					err = nil
				}
			} else {
				// Assume the message is a printable string, which it will be
				// if it's been created by the amqsput program
				fmt.Printf("Got message of length %d: ", datalen)
				fmt.Println(strings.TrimSpace(string(buffer)))
			}
		} else {
			// Create a buffer for the message data. This one is large enough
			// for the messages put by the amqsput sample.
			buffer := make([]byte, 1024)

			// Now we can try to get the message
			datalen, err = qObject.Get(getmqmd, gmo, buffer)

			if err != nil {
				msgAvail = false
				fmt.Println(err)
				mqret := err.(*ibmmq.MQReturn)
				if mqret.MQRC == ibmmq.MQRC_NO_MSG_AVAILABLE {
					err = nil
				}
			} else {
				// Assume the message is a printable string, which it will be
				// if it's been created by the amqsput program
				fmt.Printf("Got message of length %d: ", datalen)
				fmt.Println(strings.TrimSpace(string(buffer[:datalen])))
			}
		}
	}
}