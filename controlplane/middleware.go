/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 *  License, v. 2.0. If a copy of the MPL was not distributed with this
 *  file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/gilcrest/diygoapi/errs"
)

func (app *App) APIKeyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unauthorized, "Authorization header is missing"))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			errs.HTTPErrorResponse(w, app.Logger, errs.E(errs.Unauthorized, "Invalid Authorization header format"))
			return
		}

		apiKey := parts[1]

		// Store the API key in the request context
		ctx := context.WithValue(r.Context(), "api_key", apiKey)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	}
}

func (app *App) VersionHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(VersionHeader, Version)
		next.ServeHTTP(w, r)
	})
}
