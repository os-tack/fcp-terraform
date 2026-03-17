package terraform

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/os-tack/fcp-terraform/internal/fcpcore"
)

// Dispatch routes a ParsedOp to the correct handler.
func Dispatch(op *fcpcore.ParsedOp, model *TerraformModel) string {
	switch op.Verb {
	case "add":
		return handleAdd(op, model)
	case "set":
		return handleSet(op, model)
	case "unset":
		return handleUnset(op, model)
	case "remove":
		return handleRemove(op, model)
	case "connect":
		return handleConnect(op, model)
	case "disconnect":
		return handleDisconnect(op, model)
	case "label":
		return handleLabel(op, model)
	case "style":
		return handleStyle(op, model)
	case "tag":
		return handleTag(op, model)
	case "nest":
		return handleNest(op, model)
	case "unnest":
		return handleUnnest(op, model)
	default:
		suggestion := fcpcore.Suggest(op.Verb, knownVerbs)
		msg := fmt.Sprintf("unknown verb %q", op.Verb)
		if suggestion != "" {
			msg += fmt.Sprintf(". Did you mean %q?", suggestion)
		}
		return fcpcore.FormatResult(false, msg)
	}
}

var knownVerbs = []string{"add", "set", "unset", "remove", "connect", "disconnect", "label", "style", "tag", "nest", "unnest"}

// ── add handler ──────────────────────────────────────────

func handleAdd(op *fcpcore.ParsedOp, model *TerraformModel) string {
	if len(op.Positionals) == 0 {
		return fcpcore.FormatResult(false, "add requires a sub-type: resource, provider, variable, output, data, module")
	}

	subKind := strings.ToLower(op.Positionals[0])

	switch subKind {
	case "resource":
		if len(op.Positionals) < 3 {
			return fcpcore.FormatResult(false, "add resource requires TYPE and LABEL")
		}
		fullType := op.Positionals[1]
		label := op.Positionals[2]
		ref, err := model.AddResource(fullType, label, op.Params, op.QuotedParams)
		if err != nil {
			return fcpcore.FormatResult(false, err.Error())
		}
		attrs := formatAttrKeys(op.Params)
		msg := fmt.Sprintf("resource %s.%s", ref.FullType, ref.Label)
		if attrs != "" {
			msg += " (" + attrs + ")"
		}
		return fcpcore.FormatResult(true, msg, "+")

	case "provider":
		if len(op.Positionals) < 2 {
			return fcpcore.FormatResult(false, "add provider requires PROVIDER name")
		}
		name := op.Positionals[1]
		_, err := model.AddProvider(name, op.Params)
		if err != nil {
			return fcpcore.FormatResult(false, err.Error())
		}
		return fcpcore.FormatResult(true, fmt.Sprintf("provider %q", name), "+")

	case "variable":
		if len(op.Positionals) < 2 {
			return fcpcore.FormatResult(false, "add variable requires NAME")
		}
		label := op.Positionals[1]
		_, err := model.AddVariable(label, op.Params)
		if err != nil {
			return fcpcore.FormatResult(false, err.Error())
		}
		return fcpcore.FormatResult(true, fmt.Sprintf("variable %q", label), "+")

	case "output":
		if len(op.Positionals) < 2 {
			return fcpcore.FormatResult(false, "add output requires NAME")
		}
		label := op.Positionals[1]
		_, err := model.AddOutput(label, op.Params)
		if err != nil {
			return fcpcore.FormatResult(false, err.Error())
		}
		return fcpcore.FormatResult(true, fmt.Sprintf("output %q", label), "+")

	case "data":
		if len(op.Positionals) < 3 {
			return fcpcore.FormatResult(false, "add data requires TYPE and LABEL")
		}
		fullType := op.Positionals[1]
		label := op.Positionals[2]
		ref, err := model.AddDataSource(fullType, label, op.Params, op.QuotedParams)
		if err != nil {
			return fcpcore.FormatResult(false, err.Error())
		}
		return fcpcore.FormatResult(true, fmt.Sprintf("data %s.%s", ref.FullType, ref.Label), "+")

	case "module":
		if len(op.Positionals) < 2 {
			return fcpcore.FormatResult(false, "add module requires LABEL")
		}
		label := op.Positionals[1]
		_, err := model.AddModule(label, op.Params)
		if err != nil {
			return fcpcore.FormatResult(false, err.Error())
		}
		return fcpcore.FormatResult(true, fmt.Sprintf("module %q", label), "+")

	default:
		return fcpcore.FormatResult(false, fmt.Sprintf("unknown add type %q. Use: resource, provider, variable, output, data, module", subKind))
	}
}

// ── set handler ──────────────────────────────────────────

func handleSet(op *fcpcore.ParsedOp, model *TerraformModel) string {
	if len(op.Positionals) == 0 {
		return fcpcore.FormatResult(false, "set requires a block LABEL")
	}

	label := op.Positionals[0]
	ref := model.resolveRef(label)
	if ref == nil {
		return fcpcore.FormatResult(false, fmt.Sprintf("block %q not found", label))
	}

	// Positional expression syntax: set LABEL KEY "expression"
	if len(op.Params) == 0 && len(op.Positionals) >= 3 {
		key := op.Positionals[1]
		expr := strings.Join(op.Positionals[2:], " ")
		body := ref.Block.Body()
		// Raw expression: write as-is using raw tokens
		body.SetAttributeRaw(key, hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(expr)},
		})
		return fcpcore.FormatResult(true, fmt.Sprintf("%s: set %s (expression)", label, key), "*")
	}

	if len(op.Params) == 0 {
		return fcpcore.FormatResult(false, `set requires at least one key:value or KEY "expression"`)
	}

	err := model.SetAttributes(label, op.Params, op.QuotedParams)
	if err != nil {
		return fcpcore.FormatResult(false, err.Error())
	}

	keys := formatAttrKeys(op.Params)
	return fcpcore.FormatResult(true, fmt.Sprintf("%s: set %s", label, keys), "*")
}

// ── unset handler ────────────────────────────────────────

func handleUnset(op *fcpcore.ParsedOp, model *TerraformModel) string {
	if len(op.Positionals) == 0 {
		return fcpcore.FormatResult(false, "unset requires LABEL")
	}

	label := op.Positionals[0]
	keys := op.Positionals[1:]
	if len(keys) == 0 {
		return fcpcore.FormatResult(false, "unset requires at least one KEY")
	}

	err := model.UnsetAttributes(label, keys)
	if err != nil {
		return fcpcore.FormatResult(false, err.Error())
	}

	return fcpcore.FormatResult(true, fmt.Sprintf("%s: unset %s", label, strings.Join(keys, ", ")), "*")
}

// ── remove handler ───────────────────────────────────────

func handleRemove(op *fcpcore.ParsedOp, model *TerraformModel) string {
	// Selector-based removal
	if len(op.Selectors) > 0 {
		resolved := ResolveSelectorSet(op.Selectors, model)
		if len(resolved) == 0 {
			return fcpcore.FormatResult(false, "no blocks match selector")
		}
		// Collect labels before removal (removing modifies index)
		labels := make([]string, len(resolved))
		for i, ref := range resolved {
			labels[i] = ref.Label
		}
		for _, label := range labels {
			model.RemoveBlock(label)
		}
		return fcpcore.FormatResult(true, fmt.Sprintf("removed %d block(s)", len(labels)), "@")
	}

	// Label-based removal
	if len(op.Positionals) == 0 {
		return fcpcore.FormatResult(false, "remove requires LABEL or @selector")
	}

	label := op.Positionals[0]
	ref, err := model.RemoveBlock(label)
	if err != nil {
		return fcpcore.FormatResult(false, err.Error())
	}

	return fcpcore.FormatResult(true, fmt.Sprintf("%s %q", ref.Kind, label), "-")
}

// ── connect handler ──────────────────────────────────────

func handleConnect(op *fcpcore.ParsedOp, model *TerraformModel) string {
	arrowIdx := -1
	for i, p := range op.Positionals {
		if p == "->" {
			arrowIdx = i
			break
		}
	}
	if arrowIdx < 0 {
		return fcpcore.FormatResult(false, "connect requires SRC -> TGT")
	}

	srcLabel := strings.Join(op.Positionals[:arrowIdx], " ")
	tgtLabel := strings.Join(op.Positionals[arrowIdx+1:], " ")
	if srcLabel == "" || tgtLabel == "" {
		return fcpcore.FormatResult(false, "connect requires SRC -> TGT")
	}

	edgeLabel := op.Params["label"]
	err := model.Connect(srcLabel, tgtLabel, edgeLabel)
	if err != nil {
		return fcpcore.FormatResult(false, err.Error())
	}

	return fcpcore.FormatResult(true, fmt.Sprintf("%s -> %s", srcLabel, tgtLabel), "~")
}

// ── disconnect handler ───────────────────────────────────

func handleDisconnect(op *fcpcore.ParsedOp, model *TerraformModel) string {
	arrowIdx := -1
	for i, p := range op.Positionals {
		if p == "->" {
			arrowIdx = i
			break
		}
	}
	if arrowIdx < 0 {
		return fcpcore.FormatResult(false, "disconnect requires SRC -> TGT")
	}

	srcLabel := strings.Join(op.Positionals[:arrowIdx], " ")
	tgtLabel := strings.Join(op.Positionals[arrowIdx+1:], " ")

	err := model.Disconnect(srcLabel, tgtLabel)
	if err != nil {
		return fcpcore.FormatResult(false, err.Error())
	}

	return fcpcore.FormatResult(true, fmt.Sprintf("%s -> %s", srcLabel, tgtLabel), "-")
}

// ── label handler ────────────────────────────────────────

func handleLabel(op *fcpcore.ParsedOp, model *TerraformModel) string {
	if len(op.Positionals) < 2 {
		return fcpcore.FormatResult(false, "label requires OLD_LABEL NEW_LABEL")
	}

	oldLabel := op.Positionals[0]
	newLabel := op.Positionals[1]

	ref := model.resolveRef(oldLabel)
	if ref == nil {
		return fcpcore.FormatResult(false, fmt.Sprintf("block %q not found", oldLabel))
	}

	// Check for conflict: same type + new label
	existing := model.Index.Get(newLabel)
	if existing != nil && existing != ref && existing.FullType == ref.FullType {
		return fcpcore.FormatResult(false, fmt.Sprintf("%s %q already exists", ref.FullType, newLabel))
	}

	// Remove from index under old label
	model.Index.Remove(oldLabel)

	// Update hclwrite block labels
	switch ref.Kind {
	case "resource", "data":
		ref.Block.SetLabels([]string{ref.FullType, newLabel})
	case "variable", "output", "module":
		ref.Block.SetLabels([]string{newLabel})
	case "provider":
		// Provider label is the provider name — typically not renamed
		ref.Block.SetLabels([]string{newLabel})
		ref.FullType = newLabel
	}

	// Update ref and re-add to index
	ref.Label = newLabel
	model.Index.Add(ref)

	return fcpcore.FormatResult(true, fmt.Sprintf("%q -> %q", oldLabel, newLabel), "*")
}

// ── style handler ────────────────────────────────────────

func handleStyle(op *fcpcore.ParsedOp, model *TerraformModel) string {
	tagsStr := op.Params["tags"]
	if tagsStr == "" {
		return fcpcore.FormatResult(false, `style requires tags:"Key=Val,Key2=Val2"`)
	}

	// Selector-based styling
	if len(op.Selectors) > 0 {
		resolved := ResolveSelectorSet(op.Selectors, model)
		if len(resolved) == 0 {
			return fcpcore.FormatResult(false, "no blocks match selector")
		}
		for _, ref := range resolved {
			applyTags(ref, tagsStr)
		}
		return fcpcore.FormatResult(true, fmt.Sprintf("styled %d block(s)", len(resolved)), "@")
	}

	// Label-based styling
	if len(op.Positionals) == 0 {
		return fcpcore.FormatResult(false, "style requires LABEL or @selector")
	}

	label := op.Positionals[0]
	ref := model.resolveRef(label)
	if ref == nil {
		return fcpcore.FormatResult(false, fmt.Sprintf("block %q not found", label))
	}

	applyTags(ref, tagsStr)
	return fcpcore.FormatResult(true, fmt.Sprintf("%s: tags set", label), "*")
}

// applyTags parses "Key=Val,Key2=Val2" and applies to BlockRef.Tags.
func applyTags(ref *BlockRef, tagsStr string) {
	pairs := strings.Split(tagsStr, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		eqIdx := strings.Index(pair, "=")
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(pair[:eqIdx])
		val := strings.TrimSpace(pair[eqIdx+1:])
		if ref.Tags == nil {
			ref.Tags = make(map[string]string)
		}
		ref.Tags[key] = val
	}
}

// ── tag handler ──────────────────────────────────────────

func handleTag(op *fcpcore.ParsedOp, model *TerraformModel) string {
	if len(op.Params) == 0 {
		return fcpcore.FormatResult(false, "tag requires at least one key:value pair")
	}

	// Selector-based tagging
	if len(op.Selectors) > 0 {
		resolved := ResolveSelectorSet(op.Selectors, model)
		if len(resolved) == 0 {
			return fcpcore.FormatResult(false, "no blocks match selector")
		}
		for _, ref := range resolved {
			setTagsOnBlock(ref, op.Params, op.QuotedParams)
		}
		return fcpcore.FormatResult(true, fmt.Sprintf("tagged %d block(s)", len(resolved)), "@")
	}

	// Label-based tagging
	if len(op.Positionals) == 0 {
		return fcpcore.FormatResult(false, "tag requires LABEL or @selector")
	}

	label := op.Positionals[0]
	ref := model.resolveRef(label)
	if ref == nil {
		return fcpcore.FormatResult(false, fmt.Sprintf("block %q not found", label))
	}

	setTagsOnBlock(ref, op.Params, op.QuotedParams)
	return fcpcore.FormatResult(true, fmt.Sprintf("%s: tags set", label), "*")
}

// setTagsOnBlock writes a tags = { ... } attribute directly on the HCL block body.
func setTagsOnBlock(ref *BlockRef, params map[string]string, quotedParams map[string]bool) {
	body := ref.Block.Body()

	// Remove existing tags attribute
	body.RemoveAttribute("tags")
	// Remove existing tags block (if any)
	for _, blk := range body.Blocks() {
		if blk.Type() == "tags" {
			body.RemoveBlock(blk)
		}
	}

	// Build tags body using a temp block for proper token generation
	tempBlock := hclwrite.NewBlock("_temp", nil)
	tempBody := tempBlock.Body()

	// Sort keys for deterministic output
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		val := params[key]
		forceStr := quotedParams != nil && quotedParams[key]
		SetAttribute(tempBody, key, val, forceStr)
	}

	// Extract body tokens and wrap in { }
	bodyTokens := tempBody.BuildTokens(nil)
	var tokens hclwrite.Tokens
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrace, Bytes: []byte("{")})
	tokens = append(tokens, bodyTokens...)
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrace, Bytes: []byte("}")})
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})

	body.SetAttributeRaw("tags", tokens)

	// Update BlockRef.Tags
	if ref.Tags == nil {
		ref.Tags = make(map[string]string)
	}
	for k, v := range params {
		ref.Tags[k] = v
	}
}

// ── nest handler ─────────────────────────────────────────

func handleNest(op *fcpcore.ParsedOp, model *TerraformModel) string {
	if len(op.Positionals) < 2 {
		return fcpcore.FormatResult(false, "nest requires LABEL BLOCK_TYPE")
	}

	label := op.Positionals[0]
	blockType := op.Positionals[1]

	ref := model.resolveRef(label)
	if ref == nil {
		return fcpcore.FormatResult(false, fmt.Sprintf("block %q not found", label))
	}

	// Support nested nesting with / path separator
	parts := strings.Split(blockType, "/")
	targetBody := ref.Block.Body()

	if len(parts) > 1 {
		// Navigate through existing nested blocks
		for i := 0; i < len(parts)-1; i++ {
			found := false
			for _, blk := range targetBody.Blocks() {
				if blk.Type() == parts[i] {
					targetBody = blk.Body()
					found = true
					break
				}
			}
			if !found {
				return fcpcore.FormatResult(false, fmt.Sprintf("nested block %q not found on %q", parts[i], label))
			}
		}
		blockType = parts[len(parts)-1]
	}

	// Add nested block to target body
	nested := targetBody.AppendNewBlock(blockType, nil)
	body := nested.Body()
	for key, value := range op.Params {
		forceStr := op.QuotedParams != nil && op.QuotedParams[key]
		SetAttribute(body, key, value, forceStr)
	}

	return fcpcore.FormatResult(true, fmt.Sprintf("%s: %s block added", label, blockType), "+")
}

// ── unnest handler ───────────────────────────────────────

func handleUnnest(op *fcpcore.ParsedOp, model *TerraformModel) string {
	if len(op.Positionals) < 2 {
		return fcpcore.FormatResult(false, "unnest requires LABEL BLOCK_TYPE [INDEX]")
	}

	label := op.Positionals[0]
	blockType := op.Positionals[1]

	ref := model.resolveRef(label)
	if ref == nil {
		return fcpcore.FormatResult(false, fmt.Sprintf("block %q not found", label))
	}

	// Find all nested blocks of the given type
	var matching []*hclwrite.Block
	for _, block := range ref.Block.Body().Blocks() {
		if block.Type() == blockType {
			matching = append(matching, block)
		}
	}

	if len(matching) == 0 {
		return fcpcore.FormatResult(false, fmt.Sprintf("no %q nested block on %q", blockType, label))
	}

	// Determine which one to remove
	var target *hclwrite.Block
	if len(op.Positionals) >= 3 {
		idx, err := strconv.Atoi(op.Positionals[2])
		if err != nil || idx < 0 || idx >= len(matching) {
			return fcpcore.FormatResult(false, fmt.Sprintf("index %s out of range (0-%d)", op.Positionals[2], len(matching)-1))
		}
		target = matching[idx]
	} else {
		// Remove last block of that type
		target = matching[len(matching)-1]
	}

	ref.Block.Body().RemoveBlock(target)
	return fcpcore.FormatResult(true, fmt.Sprintf("%s: %s block removed", label, blockType), "-")
}

// ── helpers ──────────────────────────────────────────────

// formatAttrKeys returns a comma-separated list of map keys.
func formatAttrKeys(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
