package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	id string
	username string
	socket *websocket.Conn
	send chan[] byte
}

type ClientManager struct{
	broadcast chan []byte
	registered chan *Client
	unregistered chan *Client
	clients map[*Client]bool
}

type Message struct{
	Sender string `json:"sender,omitempty"`
	Receptor string	`json:"receptor,omitempty"`
	Content string `json:"content,omitempty"`
}

var manager= ClientManager{
	broadcast:make(chan []byte),
	registered: make(chan *Client),
	unregistered:make(chan *Client),
	clients: make(map[*Client]bool),
}

func (manager *ClientManager) start(){
	for{
		select{
		case conn:=<-manager.registered:
			manager.clients[conn]=true
			fmt.Printf("Se ha conectado el cliente: %s\n",conn.id)
		
		case conn:=<-manager.unregistered:
			if _,ok :=manager.clients[conn];ok{
				close(conn.send)
				delete(manager.clients,conn)
				jsonMessage,_:=json.Marshal(&Message{Content: fmt.Sprintf("/%s se ha desconectado",conn.username)})
				manager.send(jsonMessage,conn)
				fmt.Printf("Se ha desconectado el cliente: %s\n Con el nombre: %s\n",conn.id, conn.username)
			}

		case message:=<-manager.broadcast:
			for conn:=range manager.clients{
				select{
					case conn.send<-message:
					default:
						close(conn.send)
						delete(manager.clients,conn)
				}
			}
		}
	}
}

func (manager *ClientManager) send(message []byte, ignore *Client){
	for conn:=range manager.clients{
		if conn!=ignore{
			conn.send<-message
		}
	}
}

func (c *Client) read(){
	defer func(){
		manager.unregistered<-c
    c.socket.Close()
	}()

	first:=true
	
	for{
		_,message,err:= c.socket.ReadMessage()
		if err!=nil{
			manager.unregistered <- c
			c.socket.Close()
			break
		}

		if first{
			first=false
			c.username=string(message)
			firstMessage,_:=json.Marshal(&Message{Content:fmt.Sprintf("/%s se ha conectado\n",c.username)})
			manager.send(firstMessage,c)
			continue
		}

		jsonMessage,_:=json.Marshal(&Message{
			Sender:c.username,
			Content:string(message),
		})
		manager.broadcast<-jsonMessage
	}
}

func (c *Client) write(){
	defer c.socket.Close()

	for{
		select{
		case message,ok:=<-c.send:
			if !ok{
				c.socket.WriteMessage(websocket.CloseMessage,[]byte{})
			}
			c.socket.WriteMessage(websocket.TextMessage,message)
		}
	}
}

func wsPage(res http.ResponseWriter, req *http.Request){
	conn, err := (&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}).Upgrade(res, req, nil)

	if err!=nil{
		http.NotFound(res,req)
		return
	}

	client:=&Client{
		id: uuid.New().String(),
		socket: conn,
		send: make(chan[]byte),
	}

	manager.registered<-client

	go client.read()
	go client.write()
}




func main(){
	fmt.Println("Chat con websocket iniciado en http://localhost:12345")
	go manager.start()
	http.HandleFunc("/ws",wsPage)
	port:=os.Getenv("PORT")
	if port==""{
		port="12345"
	}

	http.ListenAndServe("0.0.0.0:"+port,nil)
}




