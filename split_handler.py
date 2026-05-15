import re
import sys
import os

def split_go_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    pkg_match = re.search(r'^package \w+', content, re.MULTILINE)
    import_match = re.search(r'import \((.*?)\)', content, re.DOTALL)
    
    pkg_decl = pkg_match.group(0) if pkg_match else "package project"
    imports = import_match.group(0) if import_match else ""

    header = f"{pkg_decl}\n\n{imports}\n\n"
    
    blocks = []
    lines = content.split('\n')
    current_block = []
    brace_count = 0
    in_block = False
    block_name = ""
    block_type = ""
    
    for i, line in enumerate(lines):
        if not in_block:
            if line.startswith('//'):
                current_block.append(line)
            elif line.startswith('type ProjectHandler') or line.startswith('func NewProjectHandler') or line.startswith('func (h *ProjectHandler)'):
                in_block = True
                brace_count += line.count('{') - line.count('}')
                current_block.append(line)
                
                if line.startswith('type ProjectHandler'):
                    block_name = "ProjectHandlerStruct"
                    block_type = "service"
                elif line.startswith('func NewProjectHandler'):
                    block_name = "NewProjectHandler"
                    block_type = "service"
                else:
                    m = re.search(r'func \(h \*ProjectHandler\) (\w+)', line)
                    if m:
                        block_name = m.group(1)
            else:
                if line.strip() != "":
                    current_block.append(line)
        else:
            current_block.append(line)
            brace_count += line.count('{') - line.count('}')
            if brace_count == 0:
                in_block = False
                blocks.append({
                    "name": block_name,
                    "content": "\n".join(current_block),
                    "type": block_type
                })
                current_block = []
                block_name = ""
                block_type = ""
                
    if current_block and any(l.strip() for l in current_block):
        blocks.append({
            "name": "leftovers",
            "content": "\n".join(current_block)
        })

    grouping = {
        "handler.go": ["ProjectHandlerStruct", "NewProjectHandler", "leftovers"],
        "actions.go": ["Create", "Start", "Stop", "Restart", "Delete", "Redeploy", "RunArtisan"],
        "settings.go": ["UpdateConfig", "UpdateEnv", "GetEnv"],
        "info.go": ["GetAll", "GetOne", "GetLogs", "GetStats"]
    }
    
    files_content = {k: header for k in grouping.keys()}
    
    for block in blocks:
        placed = False
        for filename, func_names in grouping.items():
            if block["name"] in func_names:
                files_content[filename] += block["content"] + "\n\n"
                placed = True
                break
        if not placed:
            if "utils.go" not in files_content:
                files_content["utils.go"] = header
            files_content["utils.go"] += block["content"] + "\n\n"
            
    for filename, text in files_content.items():
        with open(os.path.join("backend/internal/handlers/project", filename), 'w') as f:
            f.write(text)

split_go_file("backend/internal/handlers/project/handler.go")
