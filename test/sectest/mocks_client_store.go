// Copyright 2023 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package sectest

import (
	"context"
	"fmt"
	"github.com/cisco-open/go-lanai/pkg/security"
	"github.com/cisco-open/go-lanai/pkg/security/oauth2"
	"github.com/cisco-open/go-lanai/pkg/utils"
	"time"
)

var (
	defaultClientGrantTypes = utils.NewStringSet(
		oauth2.GrantTypeClientCredentials,
		oauth2.GrantTypePassword,
		oauth2.GrantTypeAuthCode,
		oauth2.GrantTypeImplicit,
		oauth2.GrantTypeRefresh,
		oauth2.GrantTypeSwitchUser,
		oauth2.GrantTypeSwitchTenant,
		oauth2.GrantTypeSamlSSO,
	)

	defaultClientScopes = utils.NewStringSet(
		oauth2.ScopeRead, oauth2.ScopeWrite,
		oauth2.ScopeTokenDetails, oauth2.ScopeTenantHierarchy,
		oauth2.ScopeOidc, oauth2.ScopeOidcProfile, oauth2.ScopeOidcEmail,
		oauth2.ScopeOidcAddress, oauth2.ScopeOidcPhone,
	)
)

type MockedClient struct {
	MockedClientProperties
}

func (m MockedClient) ID() interface{} {
	return m.MockedClientProperties.ClientID
}

func (m MockedClient) Type() security.AccountType {
	return security.AccountTypeDefault
}

func (m MockedClient) Username() string {
	return m.MockedClientProperties.ClientID
}

func (m MockedClient) Credentials() interface{} {
	return m.MockedClientProperties.Secret
}

func (m MockedClient) Permissions() []string {
	return nil
}

func (m MockedClient) Disabled() bool {
	return false
}

func (m MockedClient) Locked() bool {
	return false
}

func (m MockedClient) UseMFA() bool {
	return false
}

func (m MockedClient) CacheableCopy() security.Account {
	cp := MockedClient{
		m.MockedClientProperties,
	}
	cp.MockedClientProperties.Secret = ""
	return cp
}

func (m MockedClient) ClientId() string {
	return m.MockedClientProperties.ClientID
}

func (m MockedClient) SecretRequired() bool {
	return len(m.MockedClientProperties.Secret) != 0
}

func (m MockedClient) Secret() string {
	return m.MockedClientProperties.Secret
}

func (m MockedClient) GrantTypes() utils.StringSet {
	if m.MockedClientProperties.GrantTypes == nil {
		return defaultClientGrantTypes
	}
	return utils.NewStringSet(m.MockedClientProperties.GrantTypes...)
}

func (m MockedClient) RedirectUris() utils.StringSet {
	return utils.NewStringSet(m.MockedClientProperties.RedirectUris...)
}

func (m MockedClient) Scopes() utils.StringSet {
	if m.MockedClientProperties.Scopes == nil {
		return defaultClientScopes
	}
	return utils.NewStringSet(m.MockedClientProperties.Scopes...)
}

func (m MockedClient) AutoApproveScopes() utils.StringSet {
	if m.MockedClientProperties.AutoApproveScopes == nil {
		return m.Scopes()
	}
	return utils.NewStringSet(m.MockedClientProperties.AutoApproveScopes...)
}

func (m MockedClient) AccessTokenValidity() time.Duration {
	return time.Duration(m.MockedClientProperties.ATValidity)
}

func (m MockedClient) RefreshTokenValidity() time.Duration {
	return time.Duration(m.MockedClientProperties.RTValidity)
}

func (m MockedClient) UseSessionTimeout() bool {
	return true
}

func (m MockedClient) AssignedTenantIds() utils.StringSet {
	return utils.NewStringSet(m.MockedClientProperties.AssignedTenantIds...)
}

func (m MockedClient) ResourceIDs() utils.StringSet {
	return utils.NewStringSet()
}

type MockedClientStore struct {
	idLookup map[string]*MockedClient
}

func NewMockedClientStore(props ...*MockedClientProperties) *MockedClientStore {
	ret := MockedClientStore{
		idLookup: map[string]*MockedClient{},
	}
	for _, v := range props {
		ret.idLookup[v.ClientID] = &MockedClient{MockedClientProperties: *v}
	}
	return &ret
}

func (s *MockedClientStore) LoadClientByClientId(_ context.Context, clientId string) (oauth2.OAuth2Client, error) {
	if c, ok := s.idLookup[clientId]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("cannot find client with client ID [%s]", clientId)
}
