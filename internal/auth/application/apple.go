package application

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type appleClaims struct {
	sub   string
	email string
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func verifyAppleToken(idToken string, jwksBody io.Reader, expectedAud string) (*appleClaims, error) {
	body, err := io.ReadAll(jwksBody)
	if err != nil {
		return nil, fmt.Errorf("read jwks: %w", err)
	}

	var keys jwks
	if err := json.Unmarshal(body, &keys); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}

	keyMap := map[string]*rsa.PublicKey{}
	for _, k := range keys.Keys {
		pub, err := jwkToRSA(k)
		if err != nil {
			continue
		}
		keyMap[k.Kid] = pub
	}

	token, err := jwt.Parse(idToken, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		pub, ok := keyMap[kid]
		if !ok {
			return nil, fmt.Errorf("unknown kid: %s", kid)
		}
		return pub, nil
	})
	if err != nil {
		return nil, err
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid apple token claims")
	}

	iss, _ := mapClaims["iss"].(string)
	if iss != "https://appleid.apple.com" {
		return nil, fmt.Errorf("unexpected iss: %s", iss)
	}
	aud, _ := mapClaims["aud"].(string)
	if expectedAud != "" && aud != expectedAud {
		return nil, fmt.Errorf("unexpected aud: %s", aud)
	}
	exp, _ := mapClaims["exp"].(float64)
	if time.Now().Unix() > int64(exp) {
		return nil, fmt.Errorf("apple token expired")
	}

	sub, _ := mapClaims["sub"].(string)
	email, _ := mapClaims["email"].(string)
	return &appleClaims{sub: sub, email: email}, nil
}

func jwkToRSA(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	eInt := 0
	for _, b := range eBytes {
		eInt = eInt<<8 + int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: eInt}, nil
}
