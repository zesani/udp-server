package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/satori/go.uuid"
)

type Client struct {
	conn                *net.UDPConn
	alive               bool
	userID              string
	userName            string
	sendingMessageQueue chan string
	receiveMessages     chan string
	prefix              string
	g                   *gocui.Gui
	room                string
	destUserID          string
}

type Message struct {
	messageType string
	userID      string
	userName    string
	content     string
	time        string
	destUserID  string
	destType    string
}

var client = Client{alive: true, sendingMessageQueue: make(chan string), receiveMessages: make(chan string), prefix: "broadcast"}
var ip = ""
var port = ""

func (c *Client) receiveMessage() {
	var buf [512]byte
	for {
		n, err := c.conn.Read(buf[0:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Fatal error:%s", err.Error())
		}
		c.receiveMessages <- string(buf[0:n])
		fmt.Println("")
	}
}

func (c *Client) printMessage(g *gocui.Gui) {
	for {
		msg := <-c.receiveMessages
		m := c.unpackMessage(msg)
		switch m.messageType {
		case "join":
			viewPrint(g, m.time+"   -------"+m.userName+"  joining-------")
		case "broadcast":
			if m.userID == c.userID {
				viewPrint(g, m.time+"   (/g)   <me>    "+m.content)
			} else {
				viewPrint(g, m.time+"   (/g)   <"+m.userName+">    "+m.content)
			}
		case "multicast":
			viewPrint(g, m.time+"   (/r)   <"+m.userName+"->"+m.destUserID+">    "+m.content)
		case "unicast":
			viewPrint(g, m.time+"   (/w)   <"+m.userName+">    "+m.content)
		case "updateList":
			viewList(g, m.content)
		}
	}
}

func viewPrint(g *gocui.Gui, msg string) {
	out, err := g.View("v2")
	if err != nil {
		fmt.Println("err")
		return
	}
	fmt.Fprintln(out, msg)
	g.Update(reset)
}

func viewList(g *gocui.Gui, msg string) {
	out, err := g.View("v1")
	if err != nil {
		fmt.Println("err")
		return
	}
	out.Clear()
	fmt.Fprintln(out, "ID      UserName      Room")
	stringArray := strings.Split(msg, "\x03")
	for index, s := range stringArray {
		if index == len(stringArray)-1 {
			continue
		}
		stringArray2 := strings.Split(s, "\x02")
		fmt.Fprintln(out, stringArray2[1]+"  "+stringArray2[0]+"  "+stringArray2[2])
	}
	g.Update(reset)
}

func (c *Client) packMessage(msg string, messageType string, destUserID string) string {
	return strings.Join([]string{c.userID, messageType, c.userName, msg, time.Now().Format("15:04:05"), destUserID}, "\x01")
}

func (c *Client) unpackMessage(msg string) (m Message) {
	stringArray := strings.Split(msg, "\x01")
	m.userID = stringArray[0]
	m.messageType = stringArray[1]
	m.userName = stringArray[2]
	m.content = stringArray[3]
	m.time = stringArray[4]
	m.destUserID = stringArray[5]
	return
}

func (c *Client) generateUi(wg *sync.WaitGroup) {
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	defer wg.Done()
	defer g.Close()
	c.g = g
	g.Highlight = true
	g.Cursor = true
	g.SelFgColor = gocui.ColorGreen
	g.SetManagerFunc(layout)
	go client.printMessage(g)
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		log.Panicln(err)
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}
	return
}

type Input struct {
	name      string
	x, y      int
	w         int
	maxLength int
}

func NewInput(name string, x, y, w, maxLength int) *Input {
	return &Input{name: name, x: x, y: y, w: w, maxLength: maxLength}
}

var input = NewInput("input", 100, 5, 40, 100)

func (i *Input) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	cx, _ := v.Cursor()
	ox, _ := v.Origin()
	limit := ox+cx+1 > i.maxLength
	switch {
	case ch != 0 && mod == 0 && !limit:
		v.EditWrite(ch)
	case key == gocui.KeySpace && !limit:
		v.EditWrite(' ')
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		v.EditDelete(true)
	case key == gocui.KeyEnter && !limit:
		msg := v.Buffer()
		if msg == "" {
			return
		}
		client.inputCheck(msg[0 : len(msg)-1])
		v.Clear()
		v.SetCursor(0, 0)
	}
}

func (c *Client) inputCheck(msg string) {
	if msg[0:1] == "/" {
		out, err := c.g.View("v4")
		if err != nil {
			return
		}
		switch msg[0:2] {
		case "/g":
			c.prefix = "broadcast"
			out.Title = "/g"
		case "/j":
			if len(msg) < 4 {
				return
			}
			if msg[2:3] == ":" {
				switch msg[3:] {
				case "game":
					c.room = "game"
					c.conn.Write([]byte(client.packMessage("game", "room", "")))
				case "learn":
					c.room = "learn"
					c.conn.Write([]byte(client.packMessage("learn", "room", "")))
				}
			}
		case "/l":
			c.room = ""
			c.conn.Write([]byte(client.packMessage("", "room", "")))
		case "/r":
			out.Title = "/r:" + c.room
			c.prefix = "multicast"
		case "/w":
			if len(msg) < 4 {
				return
			}
			if msg[2:3] == ":" {
				c.destUserID = msg[3:]
				out.Title = "/w:" + c.destUserID
				c.prefix = "unicast"
			}
		case "/q":
			os.Exit(0)
		}
	} else {
		if c.prefix == "multicast" {
			c.conn.Write([]byte(client.packMessage(msg, "multicast", c.room)))
		} else {
			pack := client.packMessage(msg, c.prefix, c.destUserID)
			c.conn.Write([]byte(pack))
			if c.prefix == "unicast" {
				m := client.unpackMessage(pack)
				viewPrint(c.g, m.time+"   (/w)   <me>    "+m.content)
			}
		}
	}
}

func setCurrentViewOnTop(g *gocui.Gui, name string) (*gocui.View, error) {
	if _, err := g.SetCurrentView(name); err != nil {
		return nil, err
	}
	return g.SetViewOnTop(name)
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("v1", 0, 0, maxX/6-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "User Lists"
		v.Wrap = true
		v.Autoscroll = true
	}
	if v, err := g.SetView("v2", maxX/6, 0, maxX-maxX/5-1, maxY-maxY/8-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Username: " + client.userName + "\t ID: " + client.userID + "\tServer: " + ip + ":" + port
		v.Wrap = true
		v.Autoscroll = true
	}
	if v, err := g.SetView("v4", maxX/6, maxY-maxY/8, maxX-maxX/5-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "/g"
		v.Editor = input
		v.Editable = true
		if _, err = setCurrentViewOnTop(g, "v4"); err != nil {
			return err
		}
	}
	if v, err := g.SetView("v3", maxX-maxX/5, 0, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = " Help "
		fmt.Fprintln(v, " ")
		fmt.Fprintln(v, "/g          Change mode global")
		fmt.Fprintln(v, "/w:[ID] Change mode unicast to user")
		fmt.Fprintln(v, "/j:[room]   Join to room")
		fmt.Fprintln(v, "/r:[room]   Change mode multicast to room")
		fmt.Fprintln(v, "/l:[room]   Left current room")
		v.Wrap = true
		v.Autoscroll = true
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func reset(g *gocui.Gui) error {
	return nil
}
func main() {
	userName := ""
	fmt.Print("Server IP Address (127.0.0.1) : ")
	fmt.Scanf("%s\n", &ip)
	fmt.Print("Server port (8080) : ")
	fmt.Scanf("%s\n", &port)
	fmt.Print("Username : ")
	fmt.Scanf("%s\n", &userName)
	if ip == "" {
		ip = "127.0.0.1"
	}
	if port == "" {
		port = "8080"
	}
	udpAddr, err := net.ResolveUDPAddr("udp4", ip+":"+port)
	uid, err := uuid.NewV4()
	if err != nil {
		return
	}
	udpAddrl, err := net.ResolveUDPAddr("udp4", ":1150")
	if err != nil {
		return
	}
	fmt.Println(udpAddrl)
	client.userName = userName
	client.userID = uid.String()[0:6]
	if client.userName == "" {
		client.userName = client.userID
	}
	// _ = udpAddrl
	client.conn, err = net.DialUDP("udp", udpAddrl, udpAddr)
	if err != nil {
		fmt.Printf("Some error %v", err)
		return
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go client.generateUi(&wg)

	client.conn.Write([]byte(client.packMessage("join", "join", "")))
	go client.receiveMessage()
	defer client.conn.Close()
	defer client.conn.Write([]byte(client.packMessage("close", "close", "")))
	wg.Wait()
}
