package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/danieljustus/symaira-memory/internal/security"
)

var (
	tokenSubject  string
	tokenDuration int
)

func init() {
	tokenCmd.AddCommand(tokenGenCmd)
	tokenCmd.AddCommand(tokenVerifyCmd)
	
	tokenGenCmd.Flags().StringVarP(&tokenSubject, "subject", "s", "extension", "Subject/client identity for this token")
	tokenGenCmd.Flags().IntVarP(&tokenDuration, "duration", "d", 8760, "Token validity duration in hours (default 1 year)")
	
	rootCmd.AddCommand(tokenCmd)
}

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage JWT authentication tokens for the HTTP REST API",
	Long:  `Generate and verify signed JWT credentials to secure HTTP endpoints against unauthorized access.`,
}

var tokenGenCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a new signed JWT authorization token",
	Run: func(cmd *cobra.Command, args []string) {
		provider := security.NewJWTProvider("")
		duration := time.Duration(tokenDuration) * time.Hour
		
		token, err := provider.GenerateToken(tokenSubject, duration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to issue token: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("⚡ JWT Token generated successfully for '%s'!\n", tokenSubject)
		fmt.Printf("  Expires: In %d hours (%s)\n", tokenDuration, time.Now().Add(duration).Format("2006-01-02 15:04"))
		fmt.Println("\n========================== AUTHENTICATION TOKEN ==========================")
		fmt.Println(token)
		fmt.Println("==========================================================================")
		fmt.Println("\nAdd this token to your client headers:")
		fmt.Println("  Authorization: Bearer <token>")
	},
}

var tokenVerifyCmd = &cobra.Command{
	Use:   "verify [token]",
	Short: "Verify the validity and integrity of a JWT token",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		token := args[0]
		provider := security.NewJWTProvider("")
		
		payload, err := provider.VerifyToken(token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Token verification failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ Token is VALID!")
		fmt.Printf("  Subject:    %s\n", payload.Subject)
		fmt.Printf("  Issuer:     %s\n", payload.Issuer)
		fmt.Printf("  Issued At:  %s\n", time.Unix(payload.IssuedAt, 0).Format("2006-01-02 15:04:05"))
		fmt.Printf("  Expires At: %s\n", time.Unix(payload.ExpiresAt, 0).Format("2006-01-02 15:04:05"))
	},
}
