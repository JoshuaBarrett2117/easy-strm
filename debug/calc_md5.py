#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
计算密码的MD5哈希
"""

import hashlib

passwords = ['admin', 'admin123', '123456']

print("=" * 80)
print("密码MD5哈希计算：")
print("=" * 80)
for pwd in passwords:
    md5_hash = hashlib.md5(pwd.encode()).hexdigest()
    print(f"密码: {pwd}")
    print(f"MD5哈希: {md5_hash}")
    print("-" * 80)
