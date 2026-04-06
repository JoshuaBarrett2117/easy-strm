# 测试媒体源API接口
import requests
import hashlib

BASE_URL = "http://localhost:8082"

# 1. 登录获取Token
password_md5 = hashlib.md5("admin".encode()).hexdigest()
response = requests.post(
    f"{BASE_URL}/login",
    json={"name": "admin", "password": password_md5}
)

print(f"登录响应状态码: {response.status_code}")
print(f"登录响应内容: {response.text[:200]}")

data = response.json()
token = data.get("token")

if not token:
    print("登录失败")
    exit(1)

print(f"\nToken: {token[:50]}...")

# 2. 测试获取媒体源列表
headers = {"Authorization": f"Bearer {token}"}
response = requests.get(f"{BASE_URL}/media/sources", headers=headers)

print(f"\n获取媒体源列表响应状态码: {response.status_code}")
print(f"响应内容(前500字符): {response.text[:500]}")
print(f"\n响应原始字节(前50字节): {response.content[:50]}")

# 尝试解析JSON
try:
    data = response.json()
    print(f"\n解析成功:")
    print(f"state: {data.get('state')}")
    print(f"data类型: {type(data.get('data'))}")
    if isinstance(data.get('data'), dict):
        print(f"data.data类型: {type(data.get('data', {}).get('data'))}")
        print(f"data.total: {data.get('data', {}).get('total')}")
except Exception as e:
    print(f"\n解析失败: {e}")
    print(f"原始响应: {response.text}")
