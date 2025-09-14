package main

func main() {
	server := NewXcentroServer()
	server.RegisterRouters()
	server.Start(":8888")
}
