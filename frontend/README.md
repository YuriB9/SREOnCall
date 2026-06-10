# SRE OnCall — Frontend

## Требования к конфигурации Keycloak

### Groups-маппер обязателен в обоих токенах: `id_token` И `access_token`

Клиент Keycloak (`VITE_OIDC_CLIENT_ID`) должен иметь Group Membership-маппер,
который кладёт claim `groups` (полный путь группы, например `/team-alpha/admins`)
**одновременно** в `id_token` и в `access_token`. В настройках маппера Keycloak
это флаги «Add to ID token» и «Add to access token».

Причина — фронтенд и бэкенд читают группы из разных токенов:

- **фронтенд** декодирует `groups` из `id_token` (`src/auth/usePermissions.ts`,
  `user.profile`) для построения карты `tenantRoles` и навигационных гардов;
- **бэкенд** (`pkg/auth/auth.go`) валидирует `access_token` по JWKS и извлекает
  `groups` из него для проверок `IsMember`/`IsAdmin`.

Если маппер добавлен только в один из токенов, возникает тихий сбой: либо фронт
скрывает команды, к которым бэкенд даёт доступ (нет groups в `id_token`), либо
UI показывает команды, а API отвечает 403 (нет groups в `access_token`).
Признак неверной настройки на фронте: страница «Нет доступа к командам»
с диагностикой «токен содержит группы, но ни одна не распознана».

### Scope

Фронтенд запрашивает scope `openid profile email` (`src/auth/oidcConfig.ts`).
Claim `groups` **не** входит в стандартные scope — он попадает в токены только
через маппер, привязанный к клиенту напрямую или через выделенный client scope
(например, `groups`), назначенный клиенту как **Default**. Если маппер живёт в
отдельном client scope с типом Optional, его нужно либо перевести в Default,
либо добавить имя scope в строку `scope` в `oidcConfig.ts`.

Трактовка групп (зеркало серверной логики): `/{tenant}` и `/{tenant}/<подгруппа>`
дают роль `member`; ровно `/{tenant}/admins` — роль `admin`.

---

## React + TypeScript + Vite

This template provides a minimal setup to get React working in Vite with HMR and some ESLint rules.

Currently, two official plugins are available:

- [@vitejs/plugin-react](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react) uses [Oxc](https://oxc.rs)
- [@vitejs/plugin-react-swc](https://github.com/vitejs/vite-plugin-react/blob/main/packages/plugin-react-swc) uses [SWC](https://swc.rs/)

## React Compiler

The React Compiler is not enabled on this template because of its impact on dev & build performances. To add it, see [this documentation](https://react.dev/learn/react-compiler/installation).

## Expanding the ESLint configuration

If you are developing a production application, we recommend updating the configuration to enable type-aware lint rules:

```js
export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      // Other configs...

      // Remove tseslint.configs.recommended and replace with this
      tseslint.configs.recommendedTypeChecked,
      // Alternatively, use this for stricter rules
      tseslint.configs.strictTypeChecked,
      // Optionally, add this for stylistic rules
      tseslint.configs.stylisticTypeChecked,

      // Other configs...
    ],
    languageOptions: {
      parserOptions: {
        project: ['./tsconfig.node.json', './tsconfig.app.json'],
        tsconfigRootDir: import.meta.dirname,
      },
      // other options...
    },
  },
])
```

You can also install [eslint-plugin-react-x](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-x) and [eslint-plugin-react-dom](https://github.com/Rel1cx/eslint-react/tree/main/packages/plugins/eslint-plugin-react-dom) for React-specific lint rules:

```js
// eslint.config.js
import reactX from 'eslint-plugin-react-x'
import reactDom from 'eslint-plugin-react-dom'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      // Other configs...
      // Enable lint rules for React
      reactX.configs['recommended-typescript'],
      // Enable lint rules for React DOM
      reactDom.configs.recommended,
    ],
    languageOptions: {
      parserOptions: {
        project: ['./tsconfig.node.json', './tsconfig.app.json'],
        tsconfigRootDir: import.meta.dirname,
      },
      // other options...
    },
  },
])
```
