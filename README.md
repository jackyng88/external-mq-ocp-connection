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
