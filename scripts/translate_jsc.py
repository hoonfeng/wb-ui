#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
JSC C++ 头文件 → Go 文件翻译管线工具

用法:
  python translate_jsc.py translate [--force]  — 翻译下一个未翻译文件
  python translate_jsc.py done <相对路径>       — 标记文件已完成
  python translate_jsc.py next                  — 显示下一个待翻译文件
  python translate_jsc.py list                  — 显示各模块完成进度
  python translate_jsc.py watch [--force]       — 持续监控模式，自动处理下一个文件

路径映射:
  源文件: ref/WebKit/Source/JavaScriptCore/{模块}/{文件名}.h
  输出:   wb-ui/jsc/{模块小写}/{蛇形文件名}.go

模块名映射:
  API → api, builtins → builtins, bytecode → bytecode, bytecompiler → bytecompiler,
  debugger → debugger, heap → heap, inspector → inspector, interpreter → interpreter,
  parser → parser, runtime → runtime, tools → tools, wasm → wasm, yarr → yarr

文件名转换: CamelCase.h → snake_case.go
"""

import sys
import os
import re
import time

# ===== 路径配置 =====
INVENTORY_PATH = r'F:\syproject\ref\JSC_File_Inventory.md'
WEBKIT_ROOT = r'F:\syproject\ref\WebKit\Source\JavaScriptCore'
WB_UI_ROOT = r'F:\syproject\wb-ui'
JSC_OUTPUT_ROOT = os.path.join(WB_UI_ROOT, 'jsc')

# ===== 模块名映射 =====
MODULE_MAP = {
    'API': 'api',
    'builtins': 'builtins',
    'bytecode': 'bytecode',
    'bytecompiler': 'bytecompiler',
    'debugger': 'debugger',
    'heap': 'heap',
    'inspector': 'inspector',
    'interpreter': 'interpreter',
    'parser': 'parser',
    'runtime': 'runtime',
    'tools': 'tools',
    'wasm': 'wasm',
    'yarr': 'yarr',
}


def camel_to_snake(name):
    """CamelCase → snake_case 转换
    
    规则:
      - JSBase.h → js_base.go
      - APICast.h → api_cast.go
      - AccessCase.h → access_case.go
      - JSAPIGlobalObject.h → js_api_global_object.go
      - ExtraSymbolsForTAPI.h → extra_symbols_for_tapi.go
    """
    # 先处理连续大写字母（缩写词）
    # 在连续大写字母后跟小写字母时，在最后一个大写字母前加分隔符
    # e.g. "JSBase" → "JS_Base" → "js_base"
    # e.g. "JSCallback" → "JS_Callback" → "js_callback"
    # 先处理缩写词：后面跟大写字母+小写字母时拆分
    name = re.sub(r'([A-Z]+)([A-Z][a-z])', r'\1_\2', name)
    # 处理普通驼峰：小写字母后跟大写字母
    name = re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', name)
    # 转小写
    return name.lower()


def source_to_output_path(source_relative_path):
    """根据清单中的源文件相对路径，计算输出 Go 文件路径
    
    输入: 'API/APICallbackFunction.h'
    输出: ('api', 'F:\\syproject\\wb-ui\\jsc\\api\\api_callback_function.go')
    """
    parts = source_relative_path.split('/')
    module = parts[0]
    filename = parts[-1]
    
    # 确定模块小写名
    module_lower = MODULE_MAP.get(module, module.lower())
    
    # 处理子目录（如 inspector/agents/、inspector/remote/ 等）
    if len(parts) > 2:
        subdir = '/'.join(parts[1:-1])
        subdir_lower = subdir.lower()
        # 输出目录也保留子目录结构
        output_module_dir = os.path.join(JSC_OUTPUT_ROOT, module_lower, subdir_lower)
    else:
        output_module_dir = os.path.join(JSC_OUTPUT_ROOT, module_lower)
    
    # 文件名转换：去掉 .h 扩展名，CamelCase → snake_case
    basename = filename[:-2]  # 去掉 ".h"
    snake_name = camel_to_snake(basename)
    go_name = snake_name + '.go'
    
    output_path = os.path.join(output_module_dir, go_name)
    
    return module_lower, output_path


def load_inventory():
    """解析清单文件，返回文件条目列表和原始行映射"""
    with open(INVENTORY_PATH, 'r', encoding='utf-8') as f:
        content = f.read()
    
    files = []
    in_task_section = False
    
    for line in content.split('\n'):
        stripped = line.strip()
        
        # 检测翻译进度追踪章节
        if stripped == '## 翻译进度追踪':
            in_task_section = True
            continue
        
        if in_task_section:
            # 遇到下个一级标题或结束
            if stripped.startswith('## ') and stripped != '## 翻译进度追踪':
                in_task_section = False
                continue
            
            m = re.match(r'- \[( |x)\] `(.+?)`', stripped)
            if m:
                done = m.group(1) == 'x'
                path = m.group(2)
                
                # 检查源文件是否存在
                source_path = os.path.join(WEBKIT_ROOT, path.replace('/', '\\'))
                source_exists = os.path.exists(source_path)
                
                # 计算输出路径
                module_lower, output_path = source_to_output_path(path)
                output_exists = os.path.exists(output_path)
                output_size = os.path.getsize(output_path) if output_exists else 0
                
                files.append({
                    'module': module_lower,
                    'original_module': path.split('/')[0],
                    'path': path,
                    'done': done,
                    'source_exists': source_exists,
                    'source_path': source_path,
                    'output_path': output_path,
                    'output_exists': output_exists,
                    'output_size': output_size,
                    'original_line': stripped,
                })
    
    return files


def save_inventory(files):
    """将文件状态写回清单的翻译进度追踪章节"""
    with open(INVENTORY_PATH, 'r', encoding='utf-8') as f:
        content = f.read()
    
    lines = content.split('\n')
    new_lines = []
    file_idx = 0
    in_task_section = False
    task_count = 0
    
    for line in lines:
        stripped = line.strip()
        
        if stripped == '## 翻译进度追踪':
            in_task_section = True
            new_lines.append(line)
            continue
        
        if in_task_section:
            m = re.match(r'- \[( |x)\] `(.+?)`', stripped)
            if m:
                if file_idx < len(files):
                    status = 'x' if files[file_idx]['done'] else ' '
                    new_lines.append(f'- [{status}] `{files[file_idx]["path"]}`')
                    file_idx += 1
                else:
                    new_lines.append(line)
                continue
            elif stripped.startswith('### '):
                # 遇到模块标题，继续
                new_lines.append(line)
                continue
            elif stripped.startswith('## '):
                # 新的一级标题，退出翻译进度章节
                in_task_section = False
                new_lines.append(line)
                continue
            elif not stripped:
                new_lines.append(line)
                continue
            else:
                new_lines.append(line)
        else:
            new_lines.append(line)
    
    with open(INVENTORY_PATH, 'w', encoding='utf-8') as f:
        f.write('\n'.join(new_lines))


def get_next_file(files=None, force=False):
    """获取下一个需翻译的文件
    
    优先返回执行管线中的文件，然后按模块顺序返回其他文件
    """
    if files is None:
        files = load_inventory()
    
    # 执行管线优先级（按依赖顺序）
    pipeline = [
        # Phase 1: Parser
        'parser/Lexer.h', 'parser/Nodes.h', 'parser/NodeConstructors.h',
        'parser/ASTBuilder.h', 'parser/Parser.h', 'parser/SyntaxChecker.h',
        'parser/ParserArena.h', 'parser/ParserFunctionInfo.h', 'parser/ParserModes.h',
        'parser/ParserTokens.h', 'parser/ParserError.h', 'parser/ResultType.h',
        'parser/SourceCode.h', 'parser/SourceProvider.h', 'parser/SourceProviderCache.h',
        'parser/SourceProviderCacheItem.h', 'parser/UnlinkedSourceCode.h',
        'parser/VariableEnvironment.h', 'parser/ModuleAnalyzer.h', 'parser/ModuleScopeData.h',
        
        # Phase 2: BytecodeGenerator
        'bytecompiler/BytecodeGenerator.h', 'bytecompiler/BytecodeGeneratorBase.h',
        'bytecompiler/RegisterID.h', 'bytecompiler/Label.h', 'bytecompiler/LabelScope.h',
        'bytecompiler/ProfileTypeBytecodeFlag.h', 'bytecompiler/StaticPropertyAnalysis.h',
        'bytecompiler/StaticPropertyAnalyzer.h',
        
        # Phase 3: CodeBlock
        'bytecode/CodeBlock.h', 'bytecode/UnlinkedCodeBlock.h', 'bytecode/CodeBlockHash.h',
        'bytecode/Instruction.h', 'bytecode/InstructionStream.h',
        
        # Phase 4: Interpreter
        'interpreter/Interpreter.h', 'interpreter/CallFrame.h', 'interpreter/Register.h',
        'interpreter/CLoopStack.h', 'interpreter/StackVisitor.h',
    ]
    
    pipeline_files = []
    other_files = []
    
    for f in files:
        # 检查是否应该翻译
        should_translate = force or (not f['done'] and f['source_exists'])
        if not should_translate:
            continue
        
        if f['path'] in pipeline:
            pipeline_files.append(f)
        else:
            other_files.append(f)
    
    # 按管线优先级排序
    pipeline_files.sort(key=lambda f: pipeline.index(f['path']) if f['path'] in pipeline else 999)
    
    # API 模块中的文件按数字顺序排序
    other_files.sort(key=lambda f: f['path'])
    
    all_sorted = pipeline_files + other_files
    
    if all_sorted:
        return all_sorted[0]
    return None


def extract_copyright_block(content):
    """从 C++ 头文件中提取版权声明块（/* ... */ 格式）"""
    # 匹配 /* ... */ 注释块（通常在文件开头）
    m = re.match(r'/\*[\s\S]*?\*/', content)
    if m:
        return m.group(0)
    # 尝试匹配 // 风格版权声明
    lines = content.split('\n')
    copyright_lines = []
    for line in lines:
        stripped = line.strip()
        if stripped.startswith('// Copyright') or stripped.startswith('// Copyright'):
            copyright_lines.append(line)
        elif copyright_lines and stripped.startswith('//') and not stripped.startswith('// #'):
            copyright_lines.append(line)
        elif copyright_lines:
            break
    
    if copyright_lines:
        return '\n'.join(copyright_lines)
    
    return ''


def extract_body(content):
    """从 C++ 头文件中提取代码体（跳过版权声明和 #pragma once/包含卫士宏）"""
    lines = content.split('\n')
    
    # 跳过版权声明块（/* ... */）
    # 从内容中去掉 /* ... */ 块
    body = re.sub(r'/\*[\s\S]*?\*/', '', content, count=1)
    
    # 跳过 #pragma once 和空行
    body_lines = body.split('\n')
    filtered = []
    skip_header = True
    for line in body_lines:
        stripped = line.strip()
        if skip_header:
            if stripped == '' or stripped == '#pragma once' or stripped.startswith('#ifndef ') or stripped.startswith('#define ') or stripped.startswith('#endif') or stripped.startswith('#include') or stripped.startswith('WTF_'):
                continue
            skip_header = False
            filtered.append(line)
        else:
            filtered.append(line)
    
    return '\n'.join(filtered)


def copyright_block_to_go_comment(copyright_block):
    """将 C++ /* ... */ 版权声明块转换为 Go // 风格注释"""
    if not copyright_block:
        return ''
    
    lines = copyright_block.split('\n')
    go_lines = []
    
    for line in lines:
        stripped = line.strip()
        if stripped.startswith('/*'):
            # 第一行：/* 内容 → // 内容
            content = stripped[2:].strip()
            if content:
                go_lines.append(f'// {content}')
            else:
                go_lines.append('//')
        elif stripped.endswith('*/'):
            # 最后行：内容 */ → // 内容
            content = stripped[:-2].strip()
            if content:
                go_lines.append(f'// {content}')
            else:
                go_lines.append('//')
        elif stripped.startswith('*'):
            # 中间行：* 内容 → // 内容
            content = stripped[1:].strip()
            if content:
                go_lines.append(f'// {content}')
            else:
                go_lines.append('//')
        else:
            if stripped:
                go_lines.append(f'// {stripped}')
            else:
                go_lines.append('//')
    
    # 去除尾部空行
    while go_lines and go_lines[-1].strip() in ('//', ''):
        go_lines.pop()
    
    return '\n'.join(go_lines)


def license_block_to_go_comment(copyright_block):
    """将 C++ 版权块转为 Go 的 // 注释（用于 BSD/LGPL 许可证）"""
    if not copyright_block:
        return ''
    
    lines = copyright_block.split('\n')
    go_lines = []
    
    for line in lines:
        stripped = line.strip()
        if stripped.startswith('/*'):
            content = stripped[2:].strip()
            if content:
                go_lines.append(f'// {content}')
            else:
                go_lines.append('//')
        elif stripped.endswith('*/'):
            content = stripped[:-2].strip()
            if content:
                go_lines.append(f'// {content}')
            else:
                go_lines.append('//')
        elif stripped.startswith('*'):
            content = stripped[1:].strip()
            if content:
                go_lines.append(f'// {content}')
            else:
                go_lines.append('//')
        else:
            if stripped:
                go_lines.append(f'// {stripped}')
    
    return '\n'.join(go_lines)


def generate_go_skeleton(source_path, module_lower, source_relative_path):
    """从 C++ 头文件生成 Go 骨架文件
    
    返回 (go_content, error_message)
    """
    with open(source_path, 'r', encoding='utf-8', errors='replace') as f:
        content = f.read()
    
    # 提取版权声明
    copyright_block = extract_copyright_block(content)
    go_copyright = license_block_to_go_comment(copyright_block) if copyright_block else ''
    
    # 提取代码体做初步分析
    body = extract_body(content)
    
    # 检测包名: 从 MODULE_MAP 获取
    pkg_name = module_lower
    
    # 提取类型、函数、常量、变量的声明用于骨架
    # 结构体/类声明
    structs = re.findall(r'(?:typedef\s+)?(?:struct|class)\s+(\w+)', body)
    # enum class 声明
    enums = re.findall(r'(?:enum\s+(?:class\s+)?)(\w+)', body)
    # 函数声明
    funcs = re.findall(r'(\w+(?:\s*<[^>]*>)?)\s+(\w+)\s*\(([^)]*)\)\s*;', body)
    # const 常量
    consts = re.findall(r'(?:const\s+)?(\w+(?:\s*\*)?)\s+const\s+(\w+)\s*=', body)
    # #define 常量
    defines = re.findall(r'#define\s+(\w+)\s+(\S+)', body)
    # extern 声明
    externs = re.findall(r'extern\s+(?:const\s+)?(\w+(?:\s*\*)?)\s+(\w+)', body)
    
    # 构建 Go 骨架
    lines = []
    
    # 版权声明
    if go_copyright:
        lines.append(go_copyright)
        lines.append('')
    
    # 翻译说明
    lines.append(f'// 使用约定：BSD 许可证')
    lines.append(f'//')
    lines.append(f'// 从 WebKit Source/JavaScriptCore/{source_relative_path} 翻译为 Go')
    lines.append('')
    lines.append(f'package {pkg_name}')
    lines.append('')
    
    # 尝试翻译注释
    lines.append('// 此文件是 JavaScriptCore {0} 的 Go 翻译版本'.format(source_relative_path))
    lines.append('')
    
    # #define 常量 → Go const
    valid_defines = []
    for name, value in defines:
        # 跳过 include guard 宏
        if name.endswith('_h') or name.startswith('_'):
            continue
        valid_defines.append((name, value))
    
    if valid_defines:
        lines.append('// 常量定义')
        lines.append('const (')
        for name, value in valid_defines:
            lines.append(f'\t{name} = {value}')
        lines.append(')')
        lines.append('')
    
    # const 常量
    if consts:
        lines.append('// 常量')
        lines.append('const (')
        for typ, name, _ in consts:
            go_type = cpp_type_to_go_type(typ)
            lines.append(f'\t// {name} 常量')
            lines.append(f'\t{name} {go_type}')
        lines.append(')')
        lines.append('')
    
    # 枚举
    for enum_name in enums:
        lines.append(f'// {enum_name} 枚举类型')
        lines.append(f'type {enum_name} uint8')
        lines.append('')
    
    # 结构体/类 → 类型
    for struct_name in structs:
        # 跳过标准库/系统类型
        if struct_name in ('JSC', 'WTF', 'std'):
            continue
        lines.append(f'// {struct_name} 结构体定义')
        lines.append(f'type {struct_name} struct {{')
        lines.append(f'\t// TODO: 添加字段')
        lines.append('}')
        lines.append('')
        # 如果有同名函数，添加 New 函数
        lines.append(f'// New{struct_name} 创建新的 {struct_name} 实例')
        lines.append(f'func New{struct_name}() *{struct_name} {{')
        lines.append(f'\treturn &{struct_name}{{}}')
        lines.append('}')
        lines.append('')
    
    # 函数声明 → Go 函数骨架
    for ret_type, func_name, params in funcs:
        # 跳过已知系统函数和运算符
        if func_name.startswith('operator') or func_name in ('if', 'for', 'while'):
            continue
        # 跳过模板特化
        if '<' in ret_type and '>' not in ret_type:
            continue
        
        go_ret = cpp_type_to_go_type(ret_type.strip())
        go_params = parse_cpp_params(params)
        
        lines.append(f'// {funcNameToGo(func_name)} {go_ret} 函数')
        lines.append(f'func {funcNameToGo(func_name)}({go_params}) {go_ret} {{')
        lines.append(f'\t// TODO: 实现函数体')
        if go_ret != '':
            lines.append(f'\tvar zero {go_ret}')
            lines.append(f'\treturn zero')
        lines.append('}')
        lines.append('')
    
    # extern 声明
    for typ, name in externs:
        go_type = cpp_type_to_go_type(typ)
        lines.append(f'// {name} 外部变量')
        lines.append(f'var {name} {go_type}')
        lines.append('')
    
    go_content = '\n'.join(lines)
    
    return go_content, None


def cpp_type_to_go_type(cpp_type):
    """C++ 类型 → Go 类型映射"""
    type_map = {
        'void': '',
        'bool': 'bool',
        'int': 'int32',
        'unsigned': 'uint32',
        'unsigned int': 'uint32',
        'unsigned long': 'uint64',
        'unsigned long long': 'uint64',
        'long': 'int64',
        'long long': 'int64',
        'size_t': 'uintptr',
        'double': 'float64',
        'float': 'float32',
        'char': 'byte',
        'int8_t': 'int8',
        'int16_t': 'int16',
        'int32_t': 'int32',
        'int64_t': 'int64',
        'uint8_t': 'uint8',
        'uint16_t': 'uint16',
        'uint32_t': 'uint32',
        'uint64_t': 'uint64',
        'intptr_t': 'intptr',
        'uintptr_t': 'uintptr',
        'const char*': 'string',
        'const char *': 'string',
        'const char* ': 'string',
        'char*': 'string',
        'char *': 'string',
        'std::nullptr_t': 'uintptr',
    }
    
    stripped = cpp_type.strip()
    # 去除指针/引用后缀
    go_type = stripped.rstrip(' *&')
    
    if go_type in type_map:
        return type_map[go_type]
    
    # 处理指针类型
    if '*' in stripped:
        base = stripped.replace('*', '').strip()
        if base in type_map:
            base_type = type_map[base]
        else:
            base_type = base
        return f'*{base_type}' if base_type else 'uintptr'
    
    # 处理 const
    if stripped.startswith('const '):
        inner = stripped[6:].strip()
        return cpp_type_to_go_type(inner)
    
    return stripped


def parse_cpp_params(param_str):
    """解析 C++ 函数参数字符串为 Go 参数字符串"""
    if not param_str or param_str.strip() == 'void':
        return ''
    
    params = []
    depth = 0
    current = ''
    
    for ch in param_str:
        if ch in '(<':
            depth += 1
            current += ch
        elif ch in ')>':
            depth -= 1
            current += ch
        elif ch == ',' and depth == 0:
            params.append(current.strip())
            current = ''
        else:
            current += ch
    
    if current.strip():
        params.append(current.strip())
    
    go_params = []
    for i, p in enumerate(params):
        # 跳过默认值
        eq_idx = p.find('=')
        if eq_idx >= 0:
            p = p[:eq_idx].strip()
        
        # 解析类型和名称
        parts = p.split()
        if len(parts) >= 2:
            # 最后一个是变量名，前面是类型
            # 处理 const char* name 等情况
            if parts[0] == 'const' and parts[1] in ('char', 'wchar_t'):
                go_type = cpp_type_to_go_type('const char*')
                go_params.append(f'_{i} {go_type}')
            else:
                # 最后一个部分是变量名
                var_name = parts[-1].lstrip('*')
                # 跳过模板参数作为变量名的情况
                if '<' in var_name:
                    go_params.append(f'_{i} uintptr')
                else:
                    type_part = ' '.join(parts[:-1])
                    go_type = cpp_type_to_go_type(type_part)
                    go_params.append(f'{var_name} {go_type}')
        elif len(parts) == 1:
            ty = cpp_type_to_go_type(parts[0])
            if ty:
                go_params.append(f'_{i} {ty}')
            else:
                go_params.append(f'_{i} uintptr')
    
    return ', '.join(go_params)


def funcNameToGo(name):
    """C++ 函数名 → Go 函数名（保持原名，仅首字母大写供导出）"""
    if not name:
        return name
    # 如果首字母小写，首字母大写供导出
    if name[0].islower() and not name.startswith('operator'):
        # 保持原名，但如果是 get/put 类函数，保留原样
        return name
    return name


def cmd_translate(force=False):
    """执行翻译：读取下一个未翻译文件，生成 Go 骨架"""
    files = load_inventory()
    next_file = get_next_file(files, force=force)
    
    if not next_file:
        print("所有文件均已翻译完成！")
        return True
    
    # 查找也可能用 force 的模式检查已完成的文件
    if not force:
        # 检查是否还有未翻译且有源文件的
        remaining = [f for f in files if not f['done'] and f['source_exists']]
        if not remaining:
            print("所有文件均已翻译完成！")
            return True
        # 找到第一个未翻译
        target = remaining[0]
    else:
        # force 模式下，找到第一个文件（即使已标记完成）
        target = next_file
    
    source_path = target['source_path']
    output_path = target['output_path']
    source_rel = target['path']
    
    print(f"正在翻译: {source_rel}")
    print(f"  源文件: {source_path}")
    print(f"  输出:   {output_path}")
    
    if not os.path.exists(source_path):
        print(f"  错误: 源文件不存在: {source_path}")
        return False
    
    # 生成 Go 骨架
    module_name_parts = source_rel.split('/')
    module_lower = MODULE_MAP.get(module_name_parts[0], module_name_parts[0].lower())
    
    try:
        go_content, err = generate_go_skeleton(source_path, module_lower, source_rel)
    except Exception as e:
        print(f"  错误: 生成 Go 代码失败: {e}")
        return False
    
    if err:
        print(f"  错误: {err}")
        return False
    
    # 确保输出目录存在
    output_dir = os.path.dirname(output_path)
    os.makedirs(output_dir, exist_ok=True)
    
    # 如果已有文件且不 force，跳过
    if os.path.exists(output_path) and not force:
        print(f"  跳过: 输出文件已存在: {output_path}")
        # 仍然标记为完成
        for f in files:
            if f['path'] == target['path']:
                f['done'] = True
                break
        save_inventory(files)
        print(f"  已标记完成: {target['path']}")
        return True
    
    # 写入 Go 文件
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(go_content)
    
    print(f"  已写入: {output_path} ({os.path.getsize(output_path)} bytes)")
    
    # 标记为完成
    for f in files:
        if f['path'] == target['path']:
            f['done'] = True
            break
    save_inventory(files)
    print(f"  已标记完成: {target['path']}")
    
    return True


def cmd_done(path):
    """标记文件已完成"""
    files = load_inventory()
    found = False
    
    # 支持传入相对路径（含模块前缀）或完整路径
    # 规范化路径分隔符
    normalized_path = path.replace('\\', '/')
    
    # 如果传入的是完整路径，提取相对路径
    if normalized_path.startswith('F:') or normalized_path.startswith('/'):
        # 尝试提取相对部分
        for f in files:
            if f['path'] in normalized_path:
                normalized_path = f['path']
                break
    
    for f in files:
        if f['path'] == normalized_path:
            f['done'] = True
            found = True
            break
    
    if not found:
        print(f"错误: 未找到路径 '{path}'")
        print("请使用类似 'API/APICallbackFunction.h' 的格式")
        return False
    
    save_inventory(files)
    print(f"已完成: {normalized_path}")
    return True


def cmd_next():
    """显示下一个待翻译文件的信息"""
    files = load_inventory()
    next_file = get_next_file(files)
    
    if not next_file:
        print("所有文件均已翻译完成！")
        return
    
    print(f"下一个待翻译文件:")
    print(f"  清单路径:  {next_file['path']}")
    print(f"  源文件:    {next_file['source_path']}")
    print(f"  输出:      {next_file['output_path']}")
    print(f"  模块:      {next_file['module']}")
    
    # 检查文件状态
    if next_file['output_exists']:
        print(f"  状态:      已有 Go 文件 ({next_file['output_size']} bytes)，尚未标记完成")
    else:
        print(f"  状态:      未翻译")


def cmd_list():
    """显示各模块翻译进度"""
    files = load_inventory()
    total = len([f for f in files if f['source_exists']])
    done = sum(1 for f in files if f['done'])
    has_go = sum(1 for f in files if f['output_exists'])
    
    print(f"\n=== JSC 翻译进度: {done}/{total} ({done * 100 // max(total, 1)}%) ===\n")
    
    # 按模块分组
    modules = {}
    for f in files:
        m = f['module']
        if m not in modules:
            modules[m] = {'total': 0, 'done': 0, 'has_go': 0, 'size': 0}
        if f['source_exists']:
            modules[m]['total'] += 1
            if f['done']:
                modules[m]['done'] += 1
            if f['output_exists']:
                modules[m]['has_go'] += 1
                modules[m]['size'] += f['output_size']
    
    # 打印进度条
    for m in sorted(modules.keys()):
        d = modules[m]
        if d['total'] == 0:
            pct = 0
        else:
            pct = d['done'] * 100 // d['total']
        bar_len = 20
        filled = pct * bar_len // 100
        bar = '#' * filled + '-' * (bar_len - filled)
        
        size_str = f"({d['size']//1024}KB)" if d['size'] > 0 else ""
        go_str = f" go:{d['has_go']}" if d['has_go'] else ""
        print(f"  {m:16s} [{bar}] {d['done']:4d}/{d['total']:<4d} ({pct:2d}%){go_str} {size_str}")
    
    print(f"\n  总计: {done}/{total} ({done * 100 // max(total, 1)}%)")
    
    # 显示执行管线进度
    pipeline_paths = [
        'parser/Lexer.h', 'parser/Nodes.h', 'parser/NodeConstructors.h',
        'parser/ASTBuilder.h', 'parser/Parser.h', 'parser/SyntaxChecker.h',
        'parser/ParserArena.h', 'parser/ParserFunctionInfo.h', 'parser/ParserModes.h',
        'parser/ParserTokens.h', 'parser/ParserError.h', 'parser/ResultType.h',
        'parser/SourceCode.h', 'parser/SourceProvider.h', 'parser/SourceProviderCache.h',
        'parser/SourceProviderCacheItem.h', 'parser/UnlinkedSourceCode.h',
        'parser/VariableEnvironment.h', 'parser/ModuleAnalyzer.h', 'parser/ModuleScopeData.h',
        'bytecompiler/BytecodeGenerator.h', 'bytecompiler/BytecodeGeneratorBase.h',
        'bytecompiler/RegisterID.h', 'bytecompiler/Label.h', 'bytecompiler/LabelScope.h',
        'bytecompiler/ProfileTypeBytecodeFlag.h', 'bytecompiler/StaticPropertyAnalysis.h',
        'bytecompiler/StaticPropertyAnalyzer.h',
        'bytecode/CodeBlock.h', 'bytecode/UnlinkedCodeBlock.h', 'bytecode/CodeBlockHash.h',
        'bytecode/Instruction.h', 'bytecode/InstructionStream.h',
        'interpreter/Interpreter.h', 'interpreter/CallFrame.h', 'interpreter/Register.h',
        'interpreter/CLoopStack.h', 'interpreter/StackVisitor.h',
    ]
    
    pipeline_done = sum(1 for f in files if f['path'] in pipeline_paths and f['done'])
    pipeline_total = len(pipeline_paths)
    p_pct = pipeline_done * 100 // pipeline_total
    p_bar = '#' * (p_pct // 5) + '-' * (20 - p_pct // 5)
    print(f"\n  执行管线: [{p_bar}] {pipeline_done}/{pipeline_total} ({p_pct}%)")
    for p in pipeline_paths:
        for f in files:
            if f['path'] == p:
                status = 'x' if f['done'] else ' '
                has = 'GO' if f['output_exists'] else '  '
                size = f'({f["output_size"]}b)' if f['output_exists'] else ''
                print(f"    [{status}] {has} {p} {size}")
                break


def cmd_watch(force=False):
    """持续监控模式：自动处理下一个文件，等待用户确认后继续"""
    print("进入持续监控模式 (按 Ctrl+C 退出)...")
    
    while True:
        files = load_inventory()
        next_file = get_next_file(files, force=force)
        
        if not next_file:
            print("\n所有文件均已翻译完成！")
            break
        
        print(f"\n--- 下一个文件: {next_file['path']} ---")
        print(f"  源: {next_file['source_path']}")
        print(f"  输出: {next_file['output_path']}")
        
        try:
            response = input("\n按 Enter 翻译此文件，输入 's' 跳过，输入 'q' 退出: ").strip().lower()
        except (KeyboardInterrupt, EOFError):
            print("\n退出监控模式")
            break
        
        if response == 'q':
            print("退出监控模式")
            break
        elif response == 's':
            print(f"跳过: {next_file['path']}")
            # 标记跳过（但不标记完成）
            # 手动选择一个文件
            continue
        else:
            # 翻译此文件
            success = cmd_translate(force=force)
            if not success:
                print(f"翻译失败，继续下一个...")
                # 可以手动标记跳过
                for f in files:
                    if f['path'] == next_file['path']:
                        f['done'] = True
                        break
                save_inventory(files)


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)
    
    cmd = sys.argv[1]
    force = '--force' in sys.argv
    
    if cmd == 'translate':
        success = cmd_translate(force=force)
        sys.exit(0 if success else 1)
    elif cmd == 'done':
        if len(sys.argv) < 3:
            print("用法: python translate_jsc.py done <相对路径>")
            print("示例: python translate_jsc.py done 'API/APICallbackFunction.h'")
            sys.exit(1)
        success = cmd_done(sys.argv[2])
        sys.exit(0 if success else 1)
    elif cmd == 'next':
        cmd_next()
    elif cmd == 'list':
        cmd_list()
    elif cmd == 'watch':
        cmd_watch(force=force)
    else:
        print(f"未知命令: {cmd}")
        print(__doc__)
        sys.exit(1)


if __name__ == '__main__':
    main()
