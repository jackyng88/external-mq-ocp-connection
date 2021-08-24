# Connecting an External Golang Client to an IBM MQ Queue Manager on OpenShift with TLS 

## Overview
- This scenario will outline the steps required to connect an external (on your local machine) to a IBM MQ Queue Manager deployed on OpenShift Container Platform cluster. This requires a few steps such as creating a new route per SNI and channel to resolve properly.
- In this example we use Golang which uses an MQI client which requires a `.kdb` file as the certificate key repository.


## Prequisites
- OpenShift Container Platform Cluster v4.6.x
  - You will need cluster administrator access.
  - This scenario is a Cloud for Integration v2021.2.1 installation 
  - IBM MQ v9.2.0 using the IBM MQ Operator
- Golang SDK 
- IDE of your choice
- IBM MQ Development Toolkit
  - [MacOS](https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqdev/mactoolkit/)
  - [Windows](https://www-945.ibm.com/support/fixcentral/swg/selectFixes?parent=ibm~WebSphere&product=ibm/WebSphere/WebSphere+MQ&release=9.1.5&platform=Windows+64-bit,+x86&function=fixId&fixids=9.1.5.0-IBM-MQC-Win64+&useReleaseAsTarget=true&includeSupersedes=0)
  - [Linux](https://www-945.ibm.com/support/fixcentral/swg/selectFixes?parent=ibm~WebSphere&product=ibm/WebSphere/WebSphere+MQ&release=9.1.5&platform=Linux+64-bit,x86_64&function=fixId&fixids=9.1.5.0-IBM-MQC-UbuntuLinuxX64+&useReleaseAsTarget=true&includeSupersedes=0)


## Creating the Certificates
- Ideally create a separate folder your your certificates and then cd into it.
```
/Users/jackyng/Desktop/Learning/mq-golang/ssl
```

- Create the `tls.key` and `tls.crt` files with the following command. 
```
openssl req -newkey rsa:2048 -nodes -keyout tls.key -x509 -days 3650 -out tls.crt
```

- As stated in the [Prerequisites](#prequisites) you will need to have the IBM MQ Client installed to your local machine as we need access to a few of the CLI commands. The key database `.kdb` file is used as the truststore for the client application. The following command creates the `clientkey.kdb` file.
```
runmqakm -keydb -create -db clientkey.kdb -pw password -type cms -stash
```

- Now we need to add the previously public keys to the key database with the following.
```
runmqakm -cert -add -db clientkey.kdb -label mqservercert -file tls.crt -format ascii -stashed
```

## Setting up the Openshift Objects
- We will now need to create the necessary objects on the platform side. First of all is the creation of the secret that will house the certficiates. This assumes that you have already deployed the IBM MQ Operator from Cloud Pak for Integration. Create a new project if you wish.
```
oc new-project <mq> 
```

- Now create the secret.
```
oc create secret tls mq-tls-secret --key="tls.key" --cert="tls.crt"
```

- You can confirm the contents of the newly created secret like so.
```
oc get secret mq-tls-secret -o yaml
```

- Now we want to create a ConfigMap that will help run the necessary MQ commands to create a Queue named `IN` and a SVRCONN Channel named `EXT.CONN`.
```
touch mq-tls-configmap.yaml
```
- Paste the following.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mq-tls-configmap
data:
  tls.mqsc: |
    DEFINE QLOCAL('IN') REPLACE 
    DEFINE CHANNEL(EXT.CONN) CHLTYPE(SVRCONN) TRPTYPE(TCP) SSLCAUTH(OPTIONAL) SSLCIPH('ANY_TLS12_OR_HIGHER')
    SET CHLAUTH(EXT.CONN) TYPE(BLOCKUSER) USERLIST('nobody') ACTION(ADD)
```
- Apply.
```
oc apply -f mq-tls-configmap.yaml
```

- Now we need to create a new OpenShift Route for our newly created Channel. By default when you deploy a new IBM MQ Queue Manager with the IBM MQ Operator it will automatically create an OpenShift Route for you. However for external connections that is not enough. Client applications that set the SNI to the MQ channel require a new OpenShift Route to be created for each channel you wish to connect to. You also have to use unique channel names across your Red Hat OpenShift Container Platform cluster, to allow routing to the correct queue manager. You can read more about this [here](https://www.ibm.com/docs/en/ibm-mq/9.2?topic=dcqmumo-configuring-route-connect-queue-manager-from-outside-openshift-cluster)

- Create a new yaml for the Route.
```
touch mq-route.yaml
```

- Paste the contents.
```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: quickstart-ext-conn-traffic
  namespace: cp4i
spec:
  host: ext2e-conn.chl.mq.ibm.com
  to:
    kind: Service
    name: mq-tls-ibm-mq
  port:
    targetPort: 1414
  tls:
    termination: passthrough
```
- There's a few things of note here. 
  - `spec.host:` This is important. As our Channel we are going to connect to is named `EXT.CONN` this uses an Service Name Indicator (SNI) address. `EXT.CONN` is translated into `ext2e-conn` and suffixed with `.chl.mq.ibm.com`. These steps are necessary and will not properly route to the QueueManager without them. You can read more about the naming scheme [here](https://www.ibm.com/support/pages/ibm-websphere-mq-how-does-mq-provide-multiple-certificates-certlabl-capability)
  - `spec.to.name`: In this example the value is `mq-tls-ibm-mq` which will be the name of the service for the Queue Manager we will create. If you planned to create a QueueManager named `mq-tls` then this Service would be suffixed with `-ibm-mq` and become `mq-test-ibm-mq`.

- Apply.
```
oc apply -f mq-route.yaml
```

## Deploying the IBM MQ Queue Manager
- **NOTE**: In this example we use the MQSNOAUT variable to disable authorization on the queue manager. This is not recommended in a production deployment of IBM MQ and only used here for development and streamlining the process for connecting using TLS. 
- Create a new yaml file for the QueueManager CustomResource.
```
touch mq-queue-manager.yaml
```

- Paste the following contents.
```
apiVersion: mq.ibm.com/v1beta1
kind: QueueManager
metadata:
  name: mq-tls
spec:
  license:
    accept: true
    license: L-RJON-BXUPZ2
    use: NonProduction
  queueManager:
    name: QUICKSTART
    mqsc:
    - configMap:
        name: mq-tls-configmap
        items:
        - tls.mqsc
    storage:
      queueManager:
        type: ephemeral
  template:
    pod:
      containers:
        - env:
            - name: MQSNOAUT
              value: 'yes'
          name: qmgr
  version: 9.2.2.0-r1
  web:
    enabled: true
  pki:
    keys:
      - name: default
        secret:
          secretName: mq-tls-secret
          items: 
          - tls.key
          - tls.crt
```
- There are a few things of note here.
  - `metadata.name`: You can have any name here but I have chosen to go with `mq-tls`. 
  - `spec.queueManager.name`: I have the value as `QUICKSTART` which will become the name of the queue manager.
  - `spec.queueManager.mqsc.configMap`: This is where we are providing the previously created ConfigMap MQ settings into the Queue Manager.
  - `spec.queueManager.storage.type`: The value here is `ephemeral` as we are not using any persistent storage in the form of PersistentVolumes (PVs). 
  - `spec.template.pod.containers.env`: As previously mentioned at the beginning of this section we are enabling `MQSNOAUT` by setting a value to `'yes'`which makes connecting simple.
  - `spec.pki.keys.secret.secretName`: This refers to the previously created secret named `mq-tls-secret`. 
  - `spec.pki.keys.secret.secretName.items`: This is using the keys from the secret which were `tls.key` and `tls.crt` and using them for authentication.

- Apply to deploy the QueueManager CR.
```
oc apply -f mq-queue-manager.yaml
```

- Now wait for the QueueManager to be deployed. You can view the progress in your OpenShift web console or from the CLI by checking the status of the pods or the Custom Resource.
```
oc get pods

oc get qmgr mq-tls
```

- Get the URL of the route.
```
oc get route mq-tls-ibm-mq-qm -o=jsonpath='{.status.ingress[0].host}{"\n"}'
```

- It should return something like the following.
```
mq-tls-ibm-mq-qm-mq-golang.ibmcloud-roks-1234-567-0000.us-south.containers.appdomain.cloud
```

- You can either login to your newly created IBM MQ QueueManager webconsole URL with these credentials or forego the need to look at the UI.
```
oc get secret platform-auth-idp-credentials -n ibm-common-services -o=jsonpath='{.data.admin_password}' | base64 -d
```
- You can get the admin username but by default it is just `admin`. But you can form it like so.
```
oc get secret platform-auth-idp-credentials -n ibm-common-services -o=jsonpath='{.data.admin_password}' | base64 -d
```

- Through the IBM MQ web console you can generate a connection file or you can create the necessary file like so.
```
touch ccdt.json
```
- Paste the following.
```json
{
    "channel": [
      {
        "name": "EXT.CONN",
        "clientConnection": {
          "connection": [
            {
              "host": "<your-host-address>",
              "port": 443
            }
          ],
          "queueManager": "QUICKSTART"
        },
        "transmissionSecurity": {
          "cipherSpecification": "ANY_TLS12"
        },
        "type": "clientConnection"
      }
    ]
  }
```
- You will need to replace the `channel.clientConnection.host` value with the address that we got from the Route earlier.


## Testing the Connection
- Let's quickly test our settings by using `amqsputc` and `amqsgetc` from the IBM MQ Client CLI. We first need a few environment variables.
```
export MQCCDTURL='<full path to file>/ccdt.JSON'
export MQSSLKEYR='<full path to file>/clientkey'
```
- The first `MQCCDTURL` environment variable is for the connection to our queue manager/channel. The second `MQSSLKEYR` is for the `clientkey.kdb` we had created earlier for the key database.

- Let's put some messages into the queue. Run the following.
```
amqsputc IN QUICKSTART
```
- You should see something like the following.
```
Sample AMQSPUT0 start
target queue is IN

```
- Type a few messages and hit enter. When done, exit. You can now get from the queue.
```
amqsgetc IN QUICKSTART
```
- You should see your messages that you put into the queue earlier.
```
Sample AMQSGET0 start
message <asdf>
message <hi this is inside the queue>
message <1234>
```


## Connecting a Golang Application to IBM MQ
- The following Go code is based upon this repository [here](https://github.com/ibm-messaging/mq-golang/tree/master/samples).
- First we need to set a few environment variables. Run the following and replace with your values. **NOTE**: The `CONNECTION` env variable needs to be ended/suffixed with `(443)` as that's the port for the OpenShift Route.

```
export QUEUE_MANAGER=QUICKSTART \
export QUEUE=IN \
export CHANNEL=EXT.CONN \
export CONNECTION_URL=<your-ocp-route-url>(443) \
export KEY_PATH=<path-to-clientkey>/clientkey
```

- Now that we have these environment variables set let's create a basic put program. Create the Go file.
```
touch mqtlsput.go
```

- Paste the following.
```go
package main

import (
	"os"
	"strings"
	"fmt"	
	"time"
	"encoding/hex"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

func main() {

	var qMgrName string
	var qName string
	var err error
	var qMgr ibmmq.MQQueueManager
	var rc int
	var qObject ibmmq.MQObject

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

	// Open the Queue
	if err == nil {
		mqod := ibmmq.NewMQOD()
		openOptions := ibmmq.MQOO_OUTPUT

		mqod.ObjectType = ibmmq.MQOT_Q
		mqod.ObjectName = qName

		qObject, err = qMgr.Open(mqod, openOptions)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("Opened queue", qObject.Name)
		}
	}

	// PUT a message to the queue
	if err == nil {
	
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
			fmt.Println("Put message to", strings.TrimSpace(qObject.Name))
			fmt.Println("MsgId: " + hex.EncodeToString(putmqmd.MsgId))
		}
	}

	if err != nil {
		fmt.Printf("Connection to %s failed.\n", qMgrName)
		fmt.Println(err)
		rc = int(err.(*ibmmq.MQReturn).MQCC)
	}

	fmt.Println("Done.")
	qObject.Close(0)
	qMgr.Disc()
	os.Exit(rc)

}
```

- Also depending on your version of Go you're using and how you're handling GOPATH, you may or may not choose to use a `go.mod` file.
```
touch go.mod
```

```go
module golang-mq

go 1.11

require github.com/ibm-messaging/mq-golang/v5 v5.0.0

```

- You can run the PUT with the following.
```
go run mqtlsput.go
```

- You should see something similar to the below if the connection was successful as well as the PUT action.
```
Connection to QUICKSTART succeeded.
Opened queue IN
Put message to INMsgId: 414d5120515549434b535441525420206ce3236101d30540
Done.
```

- Now let's create a simple GET Go program to replicate and see what items we have previously PUT into the Queue.
```
touch mqtlsget.go
```

- Paste the following.
```go
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
```

- Some of the comments originally from the sample were left in for clarity. To run use the following.
```
go run mqtlsput.go
```
- Output should be similar to the following.
```
Connection to QUICKSTART succeeded.
Opened queue IN
Got message of length 42: Hello from Go at 2021-08-24T11:49:11-04:00
Got message of length 42: Hello from Go at 2021-08-24T12:02:45-04:00
Got message of length 42: Hello from Go at 2021-08-24T12:05:41-04:00
Got message of length 42: Hello from Go at 2021-08-24T14:18:45-04:00
MQGET: MQCC = MQCC_FAILED [2] MQRC = MQRC_NO_MSG_AVAILABLE [2033]
Done.
```


## Ending Notes
- This was only a very simple usecase outlining two MQ actions (PUT and GET) that is from a location external to the cluster. In this scenario we had a channel named `EXT.CONN` but for every new channel you will need to create a new OpenShift Route with it's own SNI mapping. 
- You can replicate other programming language applications with a `.kdb` file similar to Go as they use the MQ C library/runtime i.e. Python, Node, etc. If you plan to connect to MQ using a Java application you will need to create `.jks` keystore as well as a `.p12` truststore to connect over JMS. In this Go application our `KEY_REPOSITORY` env variable is pointing to the path with the `clientkey.kdb` file and Java JMS applications will provide paths to the `.jks` and `.p12` files instead.
- *(TO DO)* - Connecting an external Quarkus application to IBM MQ on OpenShift