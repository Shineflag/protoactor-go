package main

import (
	"flag"
	"os"
	"runtime/pprof"

	"github.com/AsynkronIT/gam/actor"

	"log"
	"sync"

	"runtime"
	"time"
)

type Msg struct {
	replyTo *actor.PID
}
type Start struct {
	Sender *actor.PID
}
type Started struct{}

type clientActor struct {
	count        int
	wgStop       *sync.WaitGroup
	messageCount int
}

func (state *clientActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *Start:
		sender := msg.Sender
		m := &Msg{
			replyTo: context.Self(),
		}
		for i := 0; i < state.messageCount; i++ {
			sender.Tell(m)
		}
	case *Msg:
		state.count++
		// if state.count%500000 == 0 {
		// 	log.Println(state.count)
		// }
		if state.count == state.messageCount {
			state.wgStop.Done()
		}
	}
}

func newLocalActor(stop *sync.WaitGroup, messageCount int) actor.Producer {
	return func() actor.Actor {
		return &clientActor{
			wgStop:       stop,
			messageCount: messageCount,
		}
	}
}

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var blockProfile = flag.String("blockprof", "", "execute contention profiling and save results here")

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// Check for lock contention profiling
	if *blockProfile != "" {
		prof, err := os.Create(*blockProfile)
		if err != nil {
			log.Fatal(err)
		}
		runtime.SetBlockProfileRate(1)
		defer func() {
			pprof.Lookup("block").WriteTo(prof, 0)
		}()
	}

	runtime.GOMAXPROCS(runtime.NumCPU())
	runtime.GC()

	var wg sync.WaitGroup

	messageCount := 10000000

	clientProps := actor.
		FromProducer(newLocalActor(&wg, messageCount)).
		WithMailbox(actor.NewUnboundedMailbox(1000))

	echoProps := actor.
		FromFunc(
			func(context actor.Context) {
				switch msg := context.Message().(type) {
				case *Msg:
					msg.replyTo.Tell(msg)
				}
			}).
		WithMailbox(actor.NewUnboundedMailbox(100000))

	clients := make([]*actor.PID, 0)
	echos := make([]*actor.PID, 0)
	clientCount := runtime.NumCPU() * 2
	for i := 0; i < clientCount; i++ {
		client := actor.Spawn(clientProps)
		echo := actor.Spawn(echoProps)
		clients = append(clients, client)
		echos = append(echos, echo)
		wg.Add(1)
	}
	start := time.Now()
	log.Println("Starting to send")

	for i := 0; i < clientCount; i++ {
		client := clients[i]
		echo := echos[i]

		client.Tell(&Start{
			Sender: echo,
		})
	}

	wg.Wait()
	elapsed := time.Since(start)
	log.Printf("Elapsed %s", elapsed)

	x := int(float32(messageCount*2*clientCount) / (float32(elapsed) / float32(time.Second)))
	log.Printf("Msg per sec %v", x)

	// f, err := os.Create("memprofile")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// pprof.WriteHeapProfile(f)
	// f.Close()
}
