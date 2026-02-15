package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	id       string
	username string
	socket   *websocket.Conn
	send     chan []byte
}

type ClientManager struct {
	broadcast    chan []byte
	registered   chan *Client
	unregistered chan *Client
	clients      map[*Client]bool
}

type Message struct {
	Sender   string `json:"sender,omitempty"`
	Receptor string `json:"receptor,omitempty"`
	Content  string `json:"content,omitempty"`
}

var manager = ClientManager{
	broadcast:    make(chan []byte),
	registered:   make(chan *Client),
	unregistered: make(chan *Client),
	clients:      make(map[*Client]bool),
}

func (manager *ClientManager) start() {
	for {
		select {
		case conn := <-manager.registered:
			manager.clients[conn] = true
			log.Printf("Cliente conectado: %s", conn.id)

		case conn := <-manager.unregistered:
			if _, ok := manager.clients[conn]; ok {
				close(conn.send)
				delete(manager.clients, conn)
				jsonMessage, err := json.Marshal(&Message{Content: fmt.Sprintf("%s se ha desconectado", conn.username)})
				if err != nil {
					log.Printf("Error al serializar mensaje de desconexión: %v", err)
				} else {
					manager.send(jsonMessage, conn)
				}
				log.Printf("Cliente desconectado: %s (username: %s)", conn.id, conn.username)
			}

		case message := <-manager.broadcast:
			for conn := range manager.clients {
				select {
				case conn.send <- message:
				default:
					close(conn.send)
					delete(manager.clients, conn)
					log.Printf("Conexión cerrada para cliente: %s", conn.id)
				}
			}
		}
	}
}

func (manager *ClientManager) send(message []byte, ignore *Client) {
	for conn := range manager.clients {
		if conn != ignore {
			conn.send <- message
		}
	}
}

func (c *Client) read() {
	defer func() {
		manager.unregistered <- c
		if err := c.socket.Close(); err != nil {
			log.Printf("Error cerrando socket del cliente %s: %v", c.id, err)
		}
	}()

	first := true

	for {
		_, message, err := c.socket.ReadMessage()
		if err != nil {
			manager.unregistered <- c
			c.socket.Close()
			break
		}

		if first {
			first = false
			c.username = string(message)
			firstMessage, err := json.Marshal(&Message{Content: fmt.Sprintf("/%s se ha conectado\n", c.username)})
			if err != nil {
				log.Printf("Error al serializar mensaje de conexión: %v", err)
				continue
			}
			manager.send(firstMessage, c)
			log.Printf("Usuario %s identificado (ID: %s)", c.username, c.id)
			continue
		}

		jsonMessage, err := json.Marshal(&Message{
			Sender:  c.username,
			Content: string(message),
		})
		if err != nil {
			log.Printf("Error al serializar mensaje del usuario %s: %v", c.username, err)
			continue
		}
		manager.broadcast <- jsonMessage
	}
}

func (c *Client) write() {
	defer func() {
		if err := c.socket.Close(); err != nil {
			log.Printf("Error cerrando socket del cliente %s: %v", c.id, err)
		}
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				if err := c.socket.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Printf("Error enviando mensaje de cierre al cliente %s: %v", c.id, err)
				}
				return
			}
			if err := c.socket.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Error enviando mensaje al cliente %s: %v", c.id, err)
				return
			}
		}
	}
}

func wsPage(res http.ResponseWriter, req *http.Request) {
	conn, err := (&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}).Upgrade(res, req, nil)

	if err != nil {
		log.Printf("Error al actualizar conexión a websocket: %v", err)
		http.NotFound(res, req)
		return
	}

	client := &Client{
		id:     uuid.New().String(),
		socket: conn,
		send:   make(chan []byte),
	}

	manager.registered <- client

	go client.read()
	go client.write()
}

func main() {
	log.Println("Iniciando servidor de chat con websocket en http://localhost:12345")
	go manager.start()
	http.HandleFunc("/ws", wsPage)
	port := os.Getenv("PORT")
	if port == "" {
		port = "12345"
	}

	log.Printf("Servidor escuchando en puerto %s", port)
	if err := http.ListenAndServe("0.0.0.0:"+port, nil); err != nil {
		log.Fatal("Error iniciando servidor: ", err)
	}
}
