import re
import sys

def split_go_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    # Find the imports and package
    pkg_match = re.search(r'^package \w+', content, re.MULTILINE)
    import_match = re.search(r'import \((.*?)\)', content, re.DOTALL)
    
    pkg_decl = pkg_match.group(0) if pkg_match else "package docker"
    imports = import_match.group(0) if import_match else ""

    header = f"{pkg_decl}\n\n{imports}\n\n"
    
    # We will use simple brace counting to extract top level blocks
    # Specifically, `type DockerService struct`, `func NewDockerService`, and `func (s *DockerService) ...`
    
    blocks = []
    
    # A simple parser
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
            elif line.startswith('type DockerService') or line.startswith('func NewDockerService') or line.startswith('func (s *DockerService)'):
                in_block = True
                brace_count += line.count('{') - line.count('}')
                current_block.append(line)
                
                if line.startswith('type DockerService'):
                    block_name = "DockerServiceStruct"
                    block_type = "service"
                elif line.startswith('func NewDockerService'):
                    block_name = "NewDockerService"
                    block_type = "service"
                else:
                    # extract function name
                    m = re.search(r'func \(s \*DockerService\) (\w+)', line)
                    if m:
                        block_name = m.group(1)
            else:
                # Top level vars or consts? Let's check
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
                
    # If there's leftovers
    if current_block and any(l.strip() for l in current_block):
        blocks.append({
            "name": "leftovers",
            "content": "\n".join(current_block)
        })

    # Grouping
    grouping = {
        "service.go": ["DockerServiceStruct", "NewDockerService", "ResolveBuildPath", "GetBuildPath", "leftovers"],
        "builder.go": ["BuildAndRun", "legacyLaravelBuild", "railpackBuild", "RunMigrations", "injectDefaultRailpackConfig", "injectDockerIgnore"],
        "lifecycle.go": ["StartWorkerContainer", "StartExistingImage", "StopContainer", "StartContainer", "RestartContainer", "RemoveContainer", "IsContainerHealthy", "ExecProjectCommand"],
        "monitor.go": ["GetLogs", "GetContainerStats", "GetAllContainerStats", "GetSystemStats", "ListAllContainers", "ListAllImages", "ListAllNetworks", "ListAllVolumes"],
        "network.go": ["DetectExposedPort"],
        "env.go": ["CreateEnvFile", "GetEnvFile", "SaveEnvFile", "loadMandatoryEnv", "parseProjectEnv"],
        "pruning.go": ["RemoveImage", "PruneImages", "CleanupProject"]
    }
    
    # Actually, we should make sure we keep any top-level consts or vars in service.go
    files_content = {k: header for k in grouping.keys()}
    
    for block in blocks:
        placed = False
        for filename, func_names in grouping.items():
            if block["name"] in func_names:
                files_content[filename] += block["content"] + "\n\n"
                placed = True
                break
        if not placed:
            # Put unknown functions in utils.go
            if "utils.go" not in files_content:
                files_content["utils.go"] = header
            files_content["utils.go"] += block["content"] + "\n\n"
            
    # Write files
    import os
    for filename, text in files_content.items():
        with open(os.path.join("backend/internal/infrastructure/docker", filename), 'w') as f:
            f.write(text)

split_go_file("backend/internal/infrastructure/docker/docker.go")
