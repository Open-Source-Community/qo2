package main

import (
	"fmt"
	"log"

	"github.com/supabase-community/gotrue-go/types"
	supabase "github.com/supabase-community/supabase-go"
)

const (
	API_URL string = "https://fzyrnorpiiahggrywtfk.supabase.co"
	API_KEY string = "sb_publishable_Jn6qoCASApvAjfhtqyIu0A_IMbOMqFt"
)

func main() {
	client, err := supabase.NewClient(
		API_URL,
		API_KEY,
		&supabase.ClientOptions{},
	)
	if err != nil {
		log.Fatalf("failed to init client: %v", err)
	}

	// Anonymous sign-in = Signup with no email/phone/password.
	resp, err := client.Auth.Signup(types.SignupRequest{})
	if err != nil {
		log.Fatalf("anonymous sign-in failed: %v", err)
	}

	fmt.Printf("Anonymous user ID: %s\n", resp.User.ID)
	fmt.Printf("Access token: %s\n", resp.Session.AccessToken)
	fmt.Printf("Refresh token: %s\n", resp.Session.RefreshToken)

	// IMPORTANT: propagate the session to the rest of the client
	// (postgrest/storage/functions) so subsequent calls are authenticated
	// as this anonymous user rather than the anon API key.
	client.UpdateAuthSession(resp.Session)

	// 3. Any database query executed through this client will automatically enforce your RLS policies
	// using the context of this specific anonymous student session!
	_, count, err := client.From("users").Select("*", "exact", false).Execute()
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	fmt.Printf("Fetched %d questions successfully under anonymous session rules.\n", count)
}
