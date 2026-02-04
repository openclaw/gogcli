package cmd

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/steipete/gogcli/internal/googleapi"
)

var newAdminDirectory = googleapi.NewAdminDirectory

// UsersCmd is the top-level users command.
type UsersCmd struct {
	List        UsersListCmd        `cmd:"" name:"list" aliases:"ls" help:"List users in domain"`
	Get         UsersGetCmd         `cmd:"" name:"get" aliases:"info" help:"Get user details"`
	Create      UsersCreateCmd      `cmd:"" name:"create" aliases:"add" help:"Create a new user"`
	Update      UsersUpdateCmd      `cmd:"" name:"update" help:"Update user attributes"`
	Delete      UsersDeleteCmd      `cmd:"" name:"delete" aliases:"rm" help:"Delete a user"`
	Suspend     UsersSuspendCmd     `cmd:"" name:"suspend" help:"Suspend a user"`
	Unsuspend   UsersUnsuspendCmd   `cmd:"" name:"unsuspend" aliases:"activate" help:"Unsuspend a user"`
	Password    UsersPasswordCmd    `cmd:"" name:"password" aliases:"passwd" help:"Reset user password"`
	Signout     UsersSignoutCmd     `cmd:"" name:"signout" help:"Sign out user from all sessions"`
	TurnOff2SV  UsersTurnOff2SVCmd  `cmd:"" name:"turnoff2sv" aliases:"disable2fa" help:"Turn off 2-step verification"`
	BackupCodes UsersBackupCodesCmd `cmd:"" name:"backupcodes" aliases:"verificationcodes" help:"Manage backup verification codes"`
	ASPs        UsersASPsCmd        `cmd:"" name:"asps" aliases:"apppasswords" help:"Manage app-specific passwords"`
	Tokens      UsersTokensCmd      `cmd:"" name:"tokens" help:"Manage user tokens"`
	Count       UsersCountCmd       `cmd:"" name:"count" help:"Count users by org unit"`
}

func extractDomain(email string) string {
	if idx := strings.LastIndex(email, "@"); idx >= 0 {
		return email[idx+1:]
	}
	return email
}

func generatePassword(length int) (string, error) {
	if length < 8 {
		length = 8
	}
	const lower = "abcdefghijklmnopqrstuvwxyz"
	const upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	const digits = "0123456789"
	const special = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	const all = lower + upper + digits + special

	sets := []string{lower, upper, digits, special}
	b := make([]byte, length)
	for i, set := range sets {
		ch, err := randChar(set)
		if err != nil {
			return "", err
		}
		b[i] = ch
	}
	for i := len(sets); i < length; i++ {
		ch, err := randChar(all)
		if err != nil {
			return "", err
		}
		b[i] = ch
	}

	for i := len(b) - 1; i > 0; i-- {
		j, err := randInt(i + 1)
		if err != nil {
			return "", err
		}
		b[i], b[j] = b[j], b[i]
	}

	return string(b), nil
}

func randChar(set string) (byte, error) {
	if len(set) == 0 {
		return 0, fmt.Errorf("empty character set")
	}
	idx, err := randInt(len(set))
	if err != nil {
		return 0, err
	}
	return set[idx], nil
}

func normalizeUserHashFunction(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "md5":
		return "MD5", nil
	case "sha-1", "sha1":
		return "SHA-1", nil
	case "crypt":
		return "crypt", nil
	case "":
		return "", nil
	default:
		return "", usage("invalid --hash-function (expected MD5, SHA-1, crypt)")
	}
}

func randInt(maxVal int) (int, error) {
	if maxVal <= 0 {
		return 0, fmt.Errorf("invalid max %d", maxVal)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxVal)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
