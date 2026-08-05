package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danieljustus/symaira-corekit/exitcodes"
	"github.com/danieljustus/symaira-memory/internal/security"
	"github.com/spf13/cobra"
)

var (
	tokenSubject  string
	tokenDuration int
)

func init() {
	tokenCmd.AddCommand(tokenGenCmd)
	tokenCmd.AddCommand(tokenVerifyCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)

	tokenGenCmd.Flags().StringVarP(&tokenSubject, "subject", "s", "extension", "Subject/client identity for this token")
	tokenGenCmd.Flags().IntVarP(&tokenDuration, "duration", "d", 72, "Token validity duration in hours (default 72h)")

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
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := security.NewJWTProvider(GetConfig(), nil)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to initialize JWT provider")
		}
		duration := time.Duration(tokenDuration) * time.Hour

		var token string
		token, err = provider.GenerateToken(tokenSubject, duration)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to generate token")
		}

		fmt.Fprintf(os.Stderr, "⚡ JWT Token generated successfully for '%s'!\n", tokenSubject)
		fmt.Fprintf(os.Stderr, "  Expires: In %d hours (%s)\n", tokenDuration, time.Now().Add(duration).Format("2006-01-02 15:04"))
		fmt.Fprintln(os.Stderr, "\n========================== AUTHENTICATION TOKEN ==========================")
		fmt.Println(token)
		fmt.Fprintln(os.Stderr, "==========================================================================")
		fmt.Fprintln(os.Stderr, "\nAdd this token to your client headers:")
		fmt.Fprintln(os.Stderr, "  Authorization: Bearer <token>")
		return nil
	},
}

var tokenVerifyCmd = &cobra.Command{
	Use:   "verify [token]",
	Short: "Verify the validity and integrity of a JWT token",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token := args[0]
		provider, err := security.NewJWTProvider(GetConfig(), nil)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to initialize JWT provider")
		}

		payload, err := provider.VerifyToken(token)
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitNoAuth, exitcodes.KindAuth, "token verification failed")
		}

		fmt.Println("✅ Token is VALID!")
		fmt.Printf("  Subject:    %s\n", payload.Subject)
		fmt.Printf("  Issuer:     %s\n", payload.Issuer)
		fmt.Printf("  Issued At:  %s\n", time.Unix(payload.IssuedAt, 0).Format("2006-01-02 15:04:05"))
		fmt.Printf("  Expires At: %s\n", time.Unix(payload.ExpiresAt, 0).Format("2006-01-02 15:04:05"))

		if database := GetDB(); database != nil && payload.Subject != "" {
			p, err := database.GetProfileByName(payload.Subject)
			if err == nil && p != nil {
				fmt.Printf("  Profile:    %s\n", p.Name)
				fmt.Printf("  Role:       %s\n", p.Role)
			} else if err == nil {
				fmt.Fprintf(os.Stderr, "  ⚠ No profile found for subject %q — default role applies\n", payload.Subject)
			}
		}
		return nil
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke [token-or-jti]",
	Short: "Revoke a JWT token so it can no longer be used",
	Long:  `Revoke a JWT token by passing either the full token value or its jti. The revocation takes effect immediately in memory and is persisted to the revocation store when a database is available.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		arg := strings.TrimSpace(args[0])
		if arg == "" {
			return exitcodes.Wrapf(fmt.Errorf("token or jti is required"), exitcodes.ExitData, exitcodes.KindValidation, "invalid argument")
		}

		provider, err := security.NewJWTProvider(GetConfig(), GetDB())
		if err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "failed to initialize JWT provider")
		}

		jti := arg
		if strings.Contains(arg, ".") {
			// A full JWT was passed — extract its jti claim without requiring
			// signature verification, so expired or invalid tokens can still
			// be revoked by value.
			jti, err = security.ExtractJTI(arg)
			if err != nil {
				return exitcodes.Wrapf(err, exitcodes.ExitData, exitcodes.KindValidation, "failed to parse token")
			}
		}

		if err := provider.RevokeToken(jti); err != nil {
			return exitcodes.Wrapf(err, exitcodes.ExitSoftware, exitcodes.KindInternal, "token revoked in memory but persistence failed")
		}

		fmt.Printf("✅ Token revoked (jti: %s). It can no longer be used to authenticate.\n", jti)
		return nil
	},
}
