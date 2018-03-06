package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type Server struct {
	conn     *net.UDPConn
	messages chan string
	clients  map[string]Client
}

type Client struct {
	userName string
	userAddr *net.UDPAddr
	userID   string
	room     string
	alive    bool
}

type Message struct {
	messageType string
	userID      string
	userName    string
	content     string
	time        string
	destUserID  string
}

func (server *Server) handleMessage() {
	var buf [512]byte

	n, addr, err := server.conn.ReadFromUDP(buf[0:])
	if err != nil {
		return
	}
	msg := string(buf[0:n])
	m := server.unpackMessage(msg)
	fmt.Println(m)
	switch m.messageType {
	case "join":
		client := Client{userAddr: addr, userName: m.userName, userID: m.userID, alive: true}
		server.clients[m.userID] = client
		server.messages <- msg
		// fmt.Printf("%s %s %s joining\n", client.userName, client.userID, client.userAddr)
		server.updateList()
	case "unicast":
		server.conn.WriteToUDP([]byte(msg), server.clients[m.destUserID].userAddr)
	case "broadcast":
		server.messages <- msg
	case "multicast":
		for _, c := range server.clients {
			if c.room != m.destUserID {
				continue
			}
			_, err := server.conn.WriteToUDP([]byte(msg), c.userAddr)
			checkError(err)
		}
	case "room":
		temp := server.clients[m.userID]
		client := Client{userAddr: temp.userAddr, userName: temp.userName, userID: temp.userID, alive: temp.alive, room: m.content}
		server.clients[m.userID] = client
		server.updateList()
	case "close":
		temp := server.clients[m.userID]
		client := Client{userAddr: temp.userAddr, userName: temp.userName, userID: temp.userID, alive: false, room: temp.room}
		server.clients[m.userID] = client
		server.updateList()
	}
}

func (server *Server) updateList() {
	lists := ""
	for i := range server.clients {
		s := server.clients[i]
		if !s.alive {
			continue
		}
		lists = lists + s.userName + "\x02" + s.userID + "\x02" + s.room + "\x03"
	}
	server.messages <- server.packMessage(lists, "updateList")
}
func (c *Client) packMessage(msg string, messageType string, destUserID string) string {
	return strings.Join([]string{c.userID, messageType, c.userName, msg, time.Now().Format("15:04:05"), destUserID}, "\x01")
}
func (s *Server) packMessage(msg string, messageType string) string {
	return strings.Join([]string{"server", messageType, "server", msg, time.Now().Format("15:04:05"), ""}, "\x01")
}
func (server *Server) unpackMessage(msg string) (m Message) {
	stringArray := strings.Split(msg, "\x01")
	m.userID = stringArray[0]
	m.messageType = stringArray[1]
	m.userName = stringArray[2]
	m.content = stringArray[3]
	m.time = stringArray[4]
	m.destUserID = stringArray[5]
	return
}
func (server *Server) sendMessage() {
	for {
		msg := <-server.messages
		udpAddr, err := net.ResolveUDPAddr("udp4", "255.255.255.255:1150")
		if err != nil {
			fmt.Println(err)
		}
		// fmt.Println(udpAddr)
		_, er := server.conn.WriteToUDP([]byte(msg), udpAddr)
		checkError(er)
		// for _, c := range server.clients {
		// 	_, err := server.conn.WriteToUDP([]byte(msg), c.userAddr)
		// 	checkError(err)
		// }
	}
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error:%s", err.Error())
		os.Exit(1)
	}
}

func main() {
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	var ipV4 net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue // interface down
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			ipV4 = ip
			// fmt.Println(ip.String())
		}
	}
	port := "8080"
	protocol := "udp"
	fmt.Print("Server port (8080) : ")
	fmt.Scanf("%s", &port)
	if port == "" {
		port = "8080"
	}
	udpAddr, err := net.ResolveUDPAddr(protocol, ":"+port)
	if err != nil {
		fmt.Println("Wrong Address")
		return
	}
	server := Server{messages: make(chan string, 20), clients: make(map[string]Client, 0)}
	server.conn, err = net.ListenUDP("udp", udpAddr)
	checkError(err)
	fmt.Println("ListenUDP \t " + ipV4.String() + ":" + port)
	go server.sendMessage()
	for {
		server.handleMessage()
	}
}
