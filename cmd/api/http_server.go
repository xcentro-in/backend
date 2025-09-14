package main

func main() {
	server := NewXcentroServer()
	server.Start(":8888")
}
