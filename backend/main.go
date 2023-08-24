package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sync"

	_ "github.com/joho/godotenv/autoload"
	plaid "github.com/plaid/plaid-go/v12/plaid"
)

var client *plaid.APIClient = nil
var dbFile *os.File = nil
var db []User = []User{}
var fileMutex sync.Mutex

var allowedOrigins = []string{"http://127.0.0.1:5173", "http://localhost:5173"}

var environments = map[string]plaid.Environment{
    "sandbox":     plaid.Sandbox,
    "development": plaid.Development,
    "production":  plaid.Production,
}

func setupClient() {
    PLAID_CLIENT_ID := os.Getenv("PLAID_CLIENT_ID")
    PLAID_SECRET := os.Getenv("PLAID_SECRET")
    PLAID_ENV := os.Getenv("PLAID_ENV")

    // create Plaid client
    configuration := plaid.NewConfiguration()
    configuration.AddDefaultHeader("PLAID-CLIENT-ID", PLAID_CLIENT_ID)
    configuration.AddDefaultHeader("PLAID-SECRET", PLAID_SECRET)
    configuration.UseEnvironment(environments[PLAID_ENV])
    client = plaid.NewAPIClient(configuration)
}

func signIn(w http.ResponseWriter, req *http.Request) {
    body, err := io.ReadAll(req.Body)

    if err != nil {
        fmt.Println(err)
        fmt.Fprintln(w, err)
        return
    }

    username := string(body)
    user := getUser(username)

    if user == nil {
        createPlaidLink(w)
        return
    }

    fmt.Fprint(w, fmt.Sprintf("{ \"accessToken\": \"%s\" }", user.AccessToken))
}

func createPlaidLink(w http.ResponseWriter) {
    ctx := context.Background()

    clientName := "TestApp"
    language := "en"
    countryCodes := []plaid.CountryCode{plaid.COUNTRYCODE_CA}
    user := *plaid.NewLinkTokenCreateRequestUser("test-user")

    linkRequest := plaid.NewLinkTokenCreateRequest(clientName, language, countryCodes, user)
    linkRequest.SetProducts([]plaid.Products{plaid.PRODUCTS_TRANSACTIONS})

    response, _, err := client.PlaidApi.LinkTokenCreate(ctx).LinkTokenCreateRequest(*linkRequest).Execute()

    if err != nil {
        plaidError, toPlaidErr := plaid.ToPlaidError(err)
        if toPlaidErr == nil {
            err = fmt.Errorf("%+v", plaidError)
        }

        fmt.Fprint(w, err)
        fmt.Println(err)
        return
    }

    fmt.Fprint(w, fmt.Sprintf("{ \"linkToken\": \"%s\" }", response.LinkToken))
}

type AccessTokenRequest struct {
    PublicToken string `json:"publicToken"`
    Username string `json:"username"`
}

func getPlaidAccessToken(w http.ResponseWriter, req *http.Request) {
    ctx := context.Background()

    body, err := io.ReadAll(req.Body)

    if err != nil {
        fmt.Println(err)
        return
    }

    fmt.Printf("Trying to unmarshal data: %+v\n", string(body))
    var data AccessTokenRequest
    err = json.Unmarshal(body, &data)
    if err != nil {
        http.Error(w, "Bad Request", http.StatusBadRequest)
        return
    }

    fmt.Printf("PublicToken: %s, username: %s\n", data.PublicToken, data.Username)

    // exchange the public_token for an access_token
    exchangePublicTokenReq := plaid.NewItemPublicTokenExchangeRequest(data.PublicToken)
    exchangePublicTokenResp, _, err := client.PlaidApi.ItemPublicTokenExchange(ctx).ItemPublicTokenExchangeRequest(*exchangePublicTokenReq).Execute()

    if err != nil {
        plaidError, toPlaidErr := plaid.ToPlaidError(err)
        if toPlaidErr == nil {
            err = fmt.Errorf("%+v", plaidError)
        }

        fmt.Println(err)
        return
    }

    // These values should be saved to a persistent database and
    // associated with the currently signed-in user
    accessToken := exchangePublicTokenResp.GetAccessToken()
    // itemID := exchangePublicTokenResp.GetItemId()
    addUser(data.Username, accessToken)

    getAccountBalances(w, ctx, accessToken)
}

func requestAccountBalances(w http.ResponseWriter, req *http.Request) {
    ctx := context.Background()

    body, err := io.ReadAll(req.Body)

    if err != nil {
        fmt.Println(err)
        fmt.Fprintln(w, err)
        return
    }

    accessToken := string(body)
    getAccountBalances(w, ctx, accessToken)
}

func getAccountBalances(w http.ResponseWriter, ctx context.Context, accessToken string) {
    // Try getting accounts
    accountsGetResp, _, err := client.PlaidApi.AccountsGet(ctx).AccountsGetRequest(
        *plaid.NewAccountsGetRequest(accessToken),
        ).Execute()

    if err != nil {
        fmt.Println(err)
        return
    }

    accounts := accountsGetResp.GetAccounts()

    for _, account := range accounts {
        balances := account.GetBalances()
        current := balances.GetCurrent()
        fmt.Printf("Current Balance in %s: %f\n", account.Name, current)
        fmt.Fprintf(w, "Current Balance in %s: %f\n", account.Name, current)
    }
}

func getBaseHandler() func(http.HandlerFunc) http.HandlerFunc {
    return func(handler http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, req *http.Request) {
            // Ensure request came from allowed origin
            origin := req.Header.Get("Origin")
            secFetchMode := req.Header.Get("Sec-Fetch-Mode")
            if secFetchMode == "cors" && !slices.Contains(allowedOrigins, origin) {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }
            w.Header().Set("Access-Control-Allow-Origin", origin)
            
            // TODO: Any other standard headers worth setting like Allowed Methods and Content Type 

            handler.ServeHTTP(w, req)
        }
    }
}

type User struct {
    Username string
    AccessToken string
}

func setupDatabase() {
    f, err := os.Create("tempdb.json");
    if err != nil {
        panic(err)
    }
    dbFile = f

    bytes, err := io.ReadAll(dbFile)
    if err != nil {
        panic(err)
    }

    json.Unmarshal(bytes, &db)
}

func getUser(username string) *User {
    idx := slices.IndexFunc(db, func(user User) bool { return user.Username == username })
    if idx == -1 {
        return nil
    }

    return &db[idx]
}


func addUser(username string, accessToken string) {
    db = append(db, User{Username: username, AccessToken: accessToken})
    bytes, err := json.Marshal(db)
    if err != nil {
        fmt.Println("Failed to convert db to json")
    }

    go func() {
        fileMutex.Lock()
        dbFile.Truncate(0)
        dbFile.Write(bytes)
        fileMutex.Unlock()
    }()
}

func main() {
    setupDatabase()

    // Setup the plaid client
    setupClient()

    handler := getBaseHandler()

    http.Handle("/signIn", handler(signIn))
    http.Handle("/getAccessToken", handler(getPlaidAccessToken))
    http.Handle("/getAccounts", handler(requestAccountBalances))

    http.ListenAndServe(":8090", nil)

    defer dbFile.Close()
}
