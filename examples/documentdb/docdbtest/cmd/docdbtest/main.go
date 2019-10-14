package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// Timeout operations after N seconds
	connectTimeout   = 5
	queryTimeout     = 30
	dbUsernameEnv    = "DB_USERNAME"
	dbPasswordEnv    = "DB_PASSWORD"
	dbClusterHostEnv = "DB_HOST"

	// Which instances to read from
	readPreference           = "secondaryPreferred"
	connectionStringTemplate = "mongodb://%s:%s@%s/test?replicaSet=rs0&readpreference=%s"

	port = 8080
)

type dbInfo struct {
	host     string
	user     string
	password string
}

func main() {
	client := newDocDB()

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout*time.Second)
		defer cancel()
		err := resetDB(ctx, client)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		fmt.Fprintln(w, "Reset database")
	})
	http.HandleFunc("/insert", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout*time.Second)
		defer cancel()
		err := addDoc(ctx, client)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		fmt.Fprintln(w, "Inserted a document")
	})
	http.HandleFunc("/count", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout*time.Second)
		defer cancel()
		count, err := countDocs(ctx, client)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		fmt.Fprintf(w, "Collection has %d docs", count)
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func getInfo() (*dbInfo, error) {
	host, ok := os.LookupEnv(dbClusterHostEnv)
	if !ok {
		return nil, fmt.Errorf("%s environment variable not set", dbClusterHostEnv)
	}
	user, ok := os.LookupEnv(dbUsernameEnv)
	if !ok {
		return nil, fmt.Errorf("%s environment variable not set", dbUsernameEnv)
	}
	password, ok := os.LookupEnv(dbPasswordEnv)
	if !ok {
		return nil, fmt.Errorf("%s environment variable not set", dbPasswordEnv)
	}
	return &dbInfo{host: host, user: user, password: password}, nil
}

func newDocDB() *mongo.Client {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout*time.Second)
	defer cancel()

	//info, err := getInfo()
	//if err != nil {
	//	panic(err)
	//}

	//connectionURI := fmt.Sprintf(connectionStringTemplate, info.user, info.password, info.host, readPreference)

	//client, err := mongo.NewClient(options.Client().ApplyURI(connectionURI))
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	err = client.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to cluster: %v", err)
	}

	// Force a connection to verify our connection string
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to ping cluster: %v", err)
	}

	fmt.Println("Connected to DocumentDB!")
	return client
}

func resetDB(ctx context.Context, client *mongo.Client) error {
	collection := client.Database("test").Collection("numbers")
	return collection.Drop(ctx)
}

func addDoc(ctx context.Context, client *mongo.Client) error {
	collection := client.Database("test").Collection("numbers")
	res, err := collection.InsertOne(ctx, bson.M{"name": "pi", "value": 3.14159})
	if err != nil {
		return err
	}

	id := res.InsertedID
	log.Printf("Inserted document ID: %s", id)

	return nil
}

func countDocs(ctx context.Context, client *mongo.Client) (int64, error) {
	collection := client.Database("test").Collection("numbers")
	fmt.Println(collection.CountDocuments(ctx, nil))
	return collection.CountDocuments(ctx, options.Count())
}
