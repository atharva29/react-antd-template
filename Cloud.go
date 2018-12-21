package main

import (
	"fmt"
	"log"
	"math/cmplx"
	"net/http"
	"runtime"
	"time"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/mjibson/go-dsp/fft"
)

// This our Data structure beteween edge and cloud communication
type NodeInfo struct {
	Id         int      // Id of the device which is connected to the Edge
	Data       float64  // Data of particular device connected to Edge
	DeviceName string   // Name of the Edge
	Date_time  int64    // Date and time of data
	AllId      []string // Gives all the Ids of device connected to the Edge
	Fft        []complex128
}

var arr = []float64{2, 3, 4, 5}

var clientConnections, deleteClientConnection, dataToWeb = make(chan *websocket.Conn), make(chan *websocket.Conn), make(chan NodeInfo)

func main() {

	go mapClients()
	router := mux.NewRouter()
	router.HandleFunc("/", simpleHandler)
	router.HandleFunc("/webSocket", handleClientSocket)
	router.HandleFunc("/ws", handleEdgeSocket)
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./")))

	err := http.ListenAndServe(":4000", router)

	//	err := http.ListenAndServe(":"+os.Getenv("PORT"), router)
	if err != nil {
		panic(err)
	}
}

func handleEdgeSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal("Upgrading To Websocket Error : ", err)
		return
	}
	fmt.Println("Connected To Edge Device")
	go readEdgeConnection(conn)
}

func readEdgeConnection(conn *websocket.Conn) {
	for {
		var Msg NodeInfo
		err := conn.ReadJSON(&Msg)
		if err != nil {
			fmt.Println("Error in reading")
			return
		}
		fmt.Println("Write Web")
		dataToWeb <- Msg
	}
}

// This Function add or deletes the client connection from the Map of connection
func mapClients() {
	// This variable has the count of number of Connections
	clientConnectionNumber := 0
	// This variable maps count with client connection
	var clientConnectionsMap = make(map[int]*websocket.Conn)
	//Sending pings to client websites
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		// If a new connection is avaialble the add it to the map
		case newClientConnection := <-clientConnections:
			{
				clientConnectionsMap[clientConnectionNumber] = newClientConnection
				clientConnectionNumber++
				fmt.Println("New Client Added :: Number : ", clientConnectionNumber, "Length :: ", len(clientConnectionsMap))

			}
		// if a connection has to be deleted from the map variable
		case deleteClientConnection := <-deleteClientConnection:
			{
				for index, conn := range clientConnectionsMap {
					if conn == deleteClientConnection {
						delete(clientConnectionsMap, index)
						fmt.Println("Old Client Deleted :: Number : ", clientConnectionNumber, ", Index :: ", index, ", Length :: ", len(clientConnectionsMap))
					}
				}
			}
			// forward data coming from edge directly to web without db
		case msg := <-dataToWeb:
			{
				fmt.Println("Data - ", msg)
				for _, conn := range clientConnectionsMap {
					err := conn.WriteJSON(msg)
					if err != nil {
						fmt.Println(err)
					}
				}
			}
			//// Maintains Constant Ping with Websites's Websocket
		case <-ticker.C:
			{
				var real [4]complex128
				temp := fft.FFTReal(arr)
				for i, _ := range temp {
					real[i] = temp[i] + cmplx.Conj(temp[i])
				}
				fmt.Println(real)

				var Data1 NodeInfo
				Data1.Fft = temp

				for _, conn := range clientConnectionsMap {
					conn.WriteJSON(Data1)

					err := conn.WriteMessage(websocket.PingMessage, []byte{})
					if err != nil {
						fmt.Println("Websocket ping fail", runtime.NumGoroutine())
						return
					} else {
						fmt.Println("Successful Ping to web ")
					}
				}
			}
		}
	}
}

func simpleHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

// upgrades the client request to websocket and initializes reading and writing from connection
func handleClientSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal("Upgrading To Websocket Error : ", err)
		return
	}
	// add the new client connection to the map of connection
	clientConnections <- conn
	// read from connection, only to check if connection is alive
	go readClientSocket(conn)
	// write from connection
}

// read from connection and check if alive, if not break the connection and delete the client connection from map
func readClientSocket(conn *websocket.Conn) {
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Error in reading from Client Socket")
			deleteClientConnection <- conn
			return
		}
	}
}
