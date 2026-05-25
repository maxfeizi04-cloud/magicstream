#!/bin/bash
# =============================================================================
# MagicStream Auth API 集成测试
# 使用方法: bash test_api.sh
# 前置条件: PostgreSQL + Redis 运行中, 服务已启动在 localhost:8080
# =============================================================================

BASE="http://localhost:8080"
PASS=0
FAIL=0

green() { printf "\033[32m%s\033[0m" "$1"; }
red()   { printf "\033[31m%s\033[0m" "$1"; }

assert_status() {
    local desc="$1" expected="$2" actual="$3" body="$4"
    if [ "$actual" -eq "$expected" ]; then
        green "  PASS"; echo " | $desc"
        PASS=$((PASS+1))
    else
        red "  FAIL"; echo " | $desc (expected $expected, got $actual)"
        echo "    body: $body"
        FAIL=$((FAIL+1))
    fi
}

assert_field() {
    local desc="$1" body="$2" field="$3"
    if echo "$body" | grep -q "\"$field\""; then
        green "  PASS"; echo " | $desc"
        PASS=$((PASS+1))
    else
        red "  FAIL"; echo " | $desc (field '$field' missing)"
        echo "    body: $body"
        FAIL=$((FAIL+1))
    fi
}

echo "=========================================="
echo " MagicStream Auth API 集成测试"
echo "=========================================="
echo ""

# ---- 1. Health Check ----
echo "[1] 健康检查"
BODY=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/health")
assert_status "GET /health → 200" 200 "$BODY" ""
echo ""

# ---- 2. Register ----
echo "[2] 注册"

# 2a. 正常注册
BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"username":"testuser1","email":"test1@example.com","password":"password123"}')
HTTP=$(echo "$BODY" | tail -1)
JSON=$(echo "$BODY" | sed '$d')
assert_status "正常注册 → 200" 200 "$HTTP" "$JSON"
assert_field "返回 user 对象" "$JSON" "user"

# 2b. 校验失败 (放在速率限制测试之前，避免被限流拦截)
BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"username":"ab","email":"bad","password":"12"}')
HTTP=$(echo "$BODY" | tail -1)
JSON=$(echo "$BODY" | sed '$d')
assert_status "校验失败 → 400" 400 "$HTTP" "$JSON"

# 2c. 重复用户名
BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"username":"testuser1","email":"other@example.com","password":"password123"}')
HTTP=$(echo "$BODY" | tail -1)
JSON=$(echo "$BODY" | sed '$d')
assert_status "重复用户名 → 409" 409 "$HTTP" "$JSON"

# 2d. 重复邮箱
BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"username":"testuser2","email":"test1@example.com","password":"password123"}')
HTTP=$(echo "$BODY" | tail -1)
JSON=$(echo "$BODY" | sed '$d')
assert_status "重复邮箱 → 409" 409 "$HTTP" "$JSON"

# 2e. 速率限制测试 (此时已消耗 3 次: 2a+2b+2c，接下来第 4 次应触发 429)
echo "  速率限制测试..."
LAST=200
for i in $(seq 1 4); do
    UNIQUE="ratetest$i"
    HTTP=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/v1/auth/register" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$UNIQUE\",\"email\":\"$UNIQUE@test.com\",\"password\":\"password123\"}")
    LAST=$HTTP
done
if [ "$LAST" -eq 429 ]; then
    green "  PASS"; echo " | 注册速率限制 → 429"
    PASS=$((PASS+1))
else
    red "  FAIL"; echo " | 注册速率限制 → 429 (got $LAST)"
    FAIL=$((FAIL+1))
fi
echo ""

# ---- 3. Login ----
echo "[3] 登录"

# 3a. 正常登录 (同时获取 headers 用于 Cookie 验证，减少请求数)
FULL=$(curl -s -i -X POST "$BASE/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"test1@example.com","password":"password123"}')
HTTP=$(echo "$FULL" | grep "^HTTP" | tail -1 | awk '{print $2}')
JSON=$(echo "$FULL" | sed -n '/^{/,/^}/p')
COOKIE_HEADER=$(echo "$FULL" | grep -i 'set-cookie')

assert_status "正常登录 → 200" 200 "$HTTP" "$JSON"
assert_field "返回 access_token" "$JSON" "access_token"
assert_field "返回 expires_in" "$JSON" "expires_in"

# 提取 access_token 用于后续测试
ACCESS_TOKEN=$(echo "$JSON" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)
echo "  access_token: ${ACCESS_TOKEN:0:20}..."

# 3b. Cookie 验证 (使用 3a 响应中的 headers)
if echo "$COOKIE_HEADER" | grep -q "refresh_token"; then
    green "  PASS"; echo " | Set-Cookie 包含 refresh_token"
    PASS=$((PASS+1))
else
    red "  FAIL"; echo " | Set-Cookie 缺少 refresh_token"
    FAIL=$((FAIL+1))
fi
if echo "$COOKIE_HEADER" | grep -q "HttpOnly"; then
    green "  PASS"; echo " | refresh_token cookie 设置了 HttpOnly"
    PASS=$((PASS+1))
else
    red "  FAIL"; echo " | refresh_token cookie 缺少 HttpOnly"
    FAIL=$((FAIL+1))
fi

# 3c. 错误密码
BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"test1@example.com","password":"wrongpass"}')
HTTP=$(echo "$BODY" | tail -1)
JSON=$(echo "$BODY" | sed '$d')
assert_status "错误密码 → 401" 401 "$HTTP" "$JSON"

# 3d. 不存在用户
BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"noone@example.com","password":"anything"}')
HTTP=$(echo "$BODY" | tail -1)
JSON=$(echo "$BODY" | sed '$d')
assert_status "不存在用户 → 401 (不区分用户不存在/密码错误)" 401 "$HTTP" "$JSON"
echo ""

# ---- 4. Refresh Token ----
echo "[4] Token 刷新"

# 4a. 登录获取 refresh token 用于 Cookie 方式刷新
FULL=$(curl -s -i -X POST "$BASE/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"test1@example.com","password":"password123"}')
COOKIE_LINE=$(echo "$FULL" | grep -i 'set-cookie' | head -1)
REFRESH_TOKEN=$(echo "$COOKIE_LINE" | grep -o 'refresh_token=[^;]*' | cut -d'=' -f2)

if [ -n "$REFRESH_TOKEN" ]; then
    # 4b. 通过 Cookie 刷新
    BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/refresh" \
        -H "Cookie: refresh_token=$REFRESH_TOKEN")
    HTTP=$(echo "$BODY" | tail -1)
    JSON=$(echo "$BODY" | sed '$d')
    assert_status "Cookie 方式刷新 → 200" 200 "$HTTP" "$JSON"
    assert_field "返回新 access_token" "$JSON" "access_token"
    assert_field "返回 expires_in" "$JSON" "expires_in"
fi

# 4c. 通过 JSON body 刷新 (移动端兼容) — 重新登录获取 fresh token
FULL2=$(curl -s -i -X POST "$BASE/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email":"test1@example.com","password":"password123"}')
COOKIE_LINE2=$(echo "$FULL2" | grep -i 'set-cookie' | head -1)
RT_JSON=$(echo "$COOKIE_LINE2" | grep -o 'refresh_token=[^;]*' | cut -d'=' -f2)

if [ -n "$RT_JSON" ]; then
    BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/refresh" \
        -H "Content-Type: application/json" \
        -d "{\"refresh_token\":\"$RT_JSON\"}")
    HTTP=$(echo "$BODY" | tail -1)
    JSON=$(echo "$BODY" | sed '$d')
    assert_status "JSON body 方式刷新 → 200" 200 "$HTTP" "$JSON"
fi

# 4d. 无效 token
BODY=$(curl -s -w "\n%{http_code}" -X POST "$BASE/api/v1/auth/refresh" \
    -H "Content-Type: application/json" \
    -d '{"refresh_token":"invalid.token.here"}')
HTTP=$(echo "$BODY" | tail -1)
JSON=$(echo "$BODY" | sed '$d')
assert_status "无效 refresh token → 401" 401 "$HTTP" "$JSON"
echo ""

# ---- 5. Protected Routes ----
echo "[5] 需认证路由"

# 5a. 无 token
BODY=$(curl -s -w "\n%{http_code}" "$BASE/api/v1/users/me")
HTTP=$(echo "$BODY" | tail -1)
assert_status "无 token 访问 /users/me → 401" 401 "$HTTP" ""

# 5b. 有效 token
if [ -n "$ACCESS_TOKEN" ]; then
    BODY=$(curl -s -w "\n%{http_code}" "$BASE/api/v1/users/me" \
        -H "Authorization: Bearer $ACCESS_TOKEN")
    HTTP=$(echo "$BODY" | tail -1)
    assert_status "有效 token 访问 /users/me → 200" 200 "$HTTP" ""
fi

# 5c. 错误 token
BODY=$(curl -s -w "\n%{http_code}" "$BASE/api/v1/users/me" \
    -H "Authorization: Bearer wrongtoken")
HTTP=$(echo "$BODY" | tail -1)
assert_status "错误 token 访问 /users/me → 401" 401 "$HTTP" ""

# 5d. 普通用户访问管理员路由
if [ -n "$ACCESS_TOKEN" ]; then
    BODY=$(curl -s -w "\n%{http_code}" "$BASE/api/v1/admin/users" \
        -H "Authorization: Bearer $ACCESS_TOKEN")
    HTTP=$(echo "$BODY" | tail -1)
    assert_status "普通用户访问 admin 路由 → 403" 403 "$HTTP" ""
fi
echo ""

# ---- Summary ----
echo "=========================================="
printf "  通过: %d  失败: %d  总计: %d\n" $PASS $FAIL $((PASS+FAIL))
if [ "$FAIL" -eq 0 ]; then
    green "  全部通过!"
else
    red "  存在失败用例"
fi
echo "=========================================="
