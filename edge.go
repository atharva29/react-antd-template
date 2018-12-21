package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3" // database
	"github.com/nathan-osman/go-rpigpio"
)

type Info struct {
	Id      []string
	Command string
}

type NodeInfo struct {
	Id         int
	Data       float64
	DeviceName string
	Date_time  int64
	AllId      []string
}

var num = 1 // auto increment of database

var CloudStep = 0
var data_to_db = make(chan NodeInfo)
var NodeInfoToCloud = make(chan NodeInfo)
var cloudwriter = make(chan string)
var NewClient = make(chan net.Conn)
var deviceID = make(chan string)
var closeCloudWriter = make(chan string)
var commandToNode = make(chan Info)

// int64 to time object
//time.Unix(sec,0)
func ResetEdge(d time.Duration) {
	var setupOfRpi = false
	for _ = range time.Tick(d) {
		p, err := rpi.OpenPin(2, rpi.OUT)
		if err != nil {
			panic(err)
		}
		defer p.Close()
		setupOfRpi = true
		if setupOfRpi == true {
			fmt.Println("RESET EDGE")
			p.Write(rpi.HIGH)
			time.Sleep(2 * time.Second)
			p.Write(rpi.LOW)
		}
	}
}

func main() {

	//	x := time.Now().UTC()
	//	sec := x.Unix()
	//fmt.Println(sec)
	// fmt.Println	fmt.Println(time.Unix(sec, 0))
	// fmt.Println(reflect.TypeOf(x))

	go ResetEdge(2 * time.Second)
	///////////////////////////////////////////////////////////////////////////

	go handleDb()
	go Mapper()
	///////////////////////////////////////////////////////////////
	listener, err := net.Listen("tcp", ":8090")
	if err != nil {
		fmt.Println(err)
	}
	go tcpServer(listener)

	dialer := websocket.Dialer{
		HandshakeTimeout: 20 * time.Second,
	}

	for {
		//conn, _, err := dialer.Dial("wss://bridge-monitoring.herokuapp.com/ws", nil)
		conn, _, err := dialer.Dial("ws://localhost:8093/ws", nil)

		if err == nil {
			CloudReader(conn)
		}
	}
}

func sensorReader(conn net.Conn) {
	step := 0
	var Msg NodeInfo
	reader := bufio.NewReader(conn)
	for {
		rawText, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Client DISCONNECTED")
			deviceID <- "DELETE"
			NewClient <- conn
			return
		} else {
			text := strings.TrimSuffix(rawText, "\n")
			temp := strings.Split(strings.TrimSpace(text), ",")
			if len(temp) == 3 {
				devId := temp[0]
				if step == 0 {
					fmt.Println("deviceID :=", devId)

					deviceID <- devId
					NewClient <- conn

					Msg.Id, err = strconv.Atoi(devId)
					if err != nil {
						fmt.Println("ID -is not integer --", devId)
					} else {
						if Msg.Data, err = strconv.ParseFloat(temp[2], 64); err != nil {
							fmt.Println("ERROR in coverion string 2 float")
						} else {
							Msg.DeviceName = temp[1]
							fmt.Println("New Id", Msg)
							data_to_db <- Msg
							if CloudStep == 1 {
								NodeInfoToCloud <- Msg
							}
							step = 1
						}
					}
				} else {
					Msg.Id, err = strconv.Atoi(devId)
					if err != nil {
						fmt.Println("ID -is not integer --", devId)
					} else {

						if Msg.Data, err = strconv.ParseFloat(temp[2], 64); err != nil {
							fmt.Println("ERROR in coverion string 2 float")
						} else {
							Msg.DeviceName = temp[1]
							data_to_db <- Msg
							if CloudStep == 1 {
								x := time.Now().UTC()
								Msg.Date_time = x.Unix()
								if CloudStep == 1 {
									NodeInfoToCloud <- Msg
								}
							}
						}
					}
				}
			} else {
				fmt.Println("Error in recieved text")
			}
		}
	}
}

func tcpServer(l net.Listener) {

	for {
		connection, err := l.Accept()
		if err != nil {
			fmt.Println(err)
		}
		go sensorReader(connection)
	}
}

func Mapper() {
	var Mapper = make(map[string]net.Conn)
	for {
		select {
		case dev := <-deviceID:
			{
				c := <-NewClient

				if dev == "DELETE" {
					for k, v := range Mapper {
						if v == c {
							delete(Mapper, k)
							fmt.Println("Deleting")
						} else {
							fmt.Println("Nothing Matched")
						}
					}

					var Msg NodeInfo
					for k, _ := range Mapper {
						Msg.AllId = append(Msg.AllId, k)
					}
					fmt.Println(Msg)
					if CloudStep == 1 {
						NodeInfoToCloud <- Msg
					}

				} else {

					Mapper[dev] = c
					fmt.Println("Nw MAp ", Mapper)
					var Msg NodeInfo
					for k, _ := range Mapper {
						Msg.AllId = append(Msg.AllId, k)
					}
					fmt.Println(Msg)
					NodeInfoToCloud <- Msg

				}
			}
		case command := <-commandToNode:
			{

				for i := 0; i < len(command.Id); i++ {
					conn, prs := Mapper[command.Id[i]]
					if prs == true {
						Writer := bufio.NewWriter(conn)
						Writer.WriteString(command.Command)
						Writer.Flush()
						fmt.Println("Msg sent to ", command.Id[i])
					} else {
						fmt.Println("NO Connection at -", command.Id[i])
					}
				}
			}
		}
	}
}

func WriteNodeInfo2Cloud(c *websocket.Conn) {
	fmt.Println(runtime.NumGoroutine())
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case Msg := <-NodeInfoToCloud:
			{
				if CloudStep == 1 {
					err := c.WriteJSON(Msg)
					if err != nil {
						fmt.Println(err)
					}
				}
			}
		case <-closeCloudWriter:
			{
				fmt.Println("break from input")
				CloudStep = 0
				return
			}
		case <-ticker.C:
			{
				if CloudStep == 1 {
					err := c.WriteMessage(websocket.PingMessage, []byte{})
					if err != nil {
						fmt.Println("Websocket ping fail", runtime.NumGoroutine())
						ticker.Stop()
						return
					} else {
						fmt.Println("Success")
						fmt.Println("GO routine - ", runtime.NumGoroutine())
					}
				}
			}
		}
	}
}

func CloudReader(conn *websocket.Conn) {

	go WriteNodeInfo2Cloud(conn)

	var Data = Info{}
	CloudStep = 1
	for {
		err := conn.ReadJSON(&Data)
		if err != nil {
			fmt.Println("read:", err)
			closeCloudWriter <- "CLOSE"
			CloudStep = 0
			return
		}
		fmt.Println(Data)
		commandToNode <- Data
	}
}

func handleDb() {

	//os.Remove("./bridge.db")
	database, _ := sql.Open("sqlite3", "./bridge.db")                                                                                                                     // creates a new db file
	statement, _ := database.Prepare("CREATE TABLE IF NOT EXISTS student (num INTEGER PRIMARY KEY,ID INT NULL,DEVICENAME TEXT NULL, DATA REAL NULL,date_time DATETIME )") //create table
	statement.Exec()                                                                                                                                                      // execute create table statement
	statement, _ = database.Prepare("INSERT INTO student (num,ID,DEVICENAME,DATA,date_time) VALUES (?,?,?,?,?)")                                                          // make statement for entering values afterwards

	statement4, _ := database.Prepare("CREATE TABLE IF NOT EXISTS counter(num INTEGER PRIMARY KEY,ID1 INTEGER NULL )") //create table
	statement4.Exec()                                                                                                  // execute create table statement
	statement4, _ = database.Prepare("INSERT INTO counter (num,ID1) VALUES (?,?)")                                     // make statement for entering values afterwards
	statement4.Exec(1, 0)

	numStatement, _ := database.Prepare("UPDATE counter SET ID1 =? WHERE num = 1;")

	rows, _ := database.Query("select * from counter where num = 1")
	var num int
	var ID1 int
	for rows.Next() {
		rows.Scan(&num, &ID1)
		fmt.Println("retrived Rows are  ID1 = ", ID1-1)
		num = ID1 + 1
	}

	point := 0

	for {
		select {
		case Msg := <-data_to_db:
			{

				statement.Exec(num, Msg.Id, Msg.DeviceName, Msg.Data, time.Now()) // put data to database

				fmt.Println(num, Msg.Id, Msg.DeviceName, Msg.Data)
				num = num + 1
				numStatement.Exec(num)

				if (CloudStep == 0) && (point == 0) {
					fmt.Println("***********point  Internet Disconnected  at ********** -  ", num)
					point = num
				}

				if (CloudStep != 0) && (point != 0) {
					rows, err := database.Query("SELECT * FROM student WHERE num >= (?) AND num < (?)", point, num)
					if err != nil {
						fmt.Println("ERROR ROWS", err)
					}
					var num int
					var date time.Time
					var Data NodeInfo

					for rows.Next() {
						fmt.Println("in rows.next for loop")
						rows.Scan(&num, &Data.Id, &Data.DeviceName, &Data.Data, &date)
						// x := time.Now().UTC()
						// sec := x.Unix()
						x := date.UTC()
						// fmt.Println("X : ", x)
						Data.Date_time = x.Unix()
						//fmt.Println(time.Unix(Data.Date_time, 0))

						NodeInfoToCloud <- Data
					}
					rows.Close()
					fmt.Println("sent over writecloud")
					point = 0
				}
			}
		}
	}
}
