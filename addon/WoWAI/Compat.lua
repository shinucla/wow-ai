-- Compatibility helpers for World of Warcraft 3.3.5a (Interface 30300).
-- Loaded before WoWAI.lua / Map.lua.

WoWAI_Compat = {}
local C = WoWAI_Compat

-- Solid-color textures: retail has SetColorTexture; 3.3.5a uses SetTexture(r,g,b,a).
function C.SetSolidColor(tex, r, g, b, a)
	a = a or 1
	if tex.SetColorTexture then
		tex:SetColorTexture(r, g, b, a)
	else
		tex:SetTexture(r, g, b, a)
	end
end

-- Frames that need SetBackdrop: no BackdropTemplate on 3.3.5a.
function C.Frame(name, parent)
	return CreateFrame("Frame", name, parent)
end

function C.SetMinResize(frame, w, h)
	if frame.SetResizeBounds then
		frame:SetResizeBounds(w, h)
	elseif frame.SetMinResize then
		frame:SetMinResize(w, h)
	end
end

-- Physical pixel height for the outbound color strip.
function C.PhysicalScreenHeight()
	if GetPhysicalScreenSize then
		local _, h = GetPhysicalScreenSize()
		if h and h > 0 then return h end
	end
	if GetCurrentResolution and GetScreenResolutions then
		local list = { GetScreenResolutions() }
		local cur = list[GetCurrentResolution()]
		if type(cur) == "string" then
			local h = cur:match("%d+x(%d+)")
			if h then return tonumber(h) end
		end
	end
	local sh = GetScreenHeight and GetScreenHeight() or 768
	local sc = UIParent and UIParent.GetEffectiveScale and UIParent:GetEffectiveScale() or 1
	return math.floor(sh * sc + 0.5)
end

-- C_Timer (missing on 3.3.5a).
if type(C_Timer) ~= "table" then
	C_Timer = {}
	local afterList, tickers = {}, {}
	local driver = CreateFrame("Frame")
	driver:SetScript("OnUpdate", function(_, elapsed)
		for i = #afterList, 1, -1 do
			local e = afterList[i]
			e.t = e.t - elapsed
			if e.t <= 0 then
				table.remove(afterList, i)
				pcall(e.fn)
			end
		end
		for i = #tickers, 1, -1 do
			local e = tickers[i]
			if e.cancelled then
				table.remove(tickers, i)
			else
				e.rem = e.rem - elapsed
				if e.rem <= 0 then
					e.rem = e.iv
					pcall(e.fn)
				end
			end
		end
	end)
	function C_Timer.After(sec, fn)
		afterList[#afterList + 1] = { t = sec or 0, fn = fn }
	end
	function C_Timer.NewTicker(sec, fn)
		local handle = { iv = sec or 1, rem = sec or 1, fn = fn, cancelled = false }
		function handle:Cancel() self.cancelled = true end
		tickers[#tickers + 1] = handle
		return handle
	end
end

if type(wipe) ~= "function" then
	function wipe(t)
		for k in pairs(t) do t[k] = nil end
		return t
	end
end

-- SetSize / SetShown arrived after 3.3.5a; polyfill on widget prototypes.
-- HookScript exists on 3.x but some private builds omit it — soft polyfill.
do
	local function addHookScript(idx)
		if idx.HookScript or type(idx.GetScript) ~= "function" then return end
		function idx:HookScript(handler, script)
			local old = self:GetScript(handler)
			if old then
				self:SetScript(handler, function(...)
					old(...)
					script(...)
				end)
			else
				self:SetScript(handler, script)
			end
		end
	end
	local function patch(obj)
		local mt = getmetatable(obj)
		if not mt or type(mt.__index) ~= "table" then return end
		local idx = mt.__index
		if not idx.SetSize and type(idx.SetWidth) == "function" then
			function idx:SetSize(w, h)
				self:SetWidth(w)
				if h then self:SetHeight(h) end
			end
		end
		if not idx.SetShown and type(idx.Show) == "function" then
			function idx:SetShown(show)
				if show then self:Show() else self:Hide() end
			end
		end
		addHookScript(idx)
	end
	-- Probe widgets only exist to read their metatable. EditBoxes default to
	-- SetAutoFocus(true) on 3.3.5a — a leaked probe steals WASD forever.
	local function discard(obj)
		if not obj then return end
		if obj.ClearFocus then pcall(obj.ClearFocus, obj) end
		if obj.EnableKeyboard then pcall(obj.EnableKeyboard, obj, false) end
		if obj.EnableMouse then pcall(obj.EnableMouse, obj, false) end
		if obj.SetAutoFocus then pcall(obj.SetAutoFocus, obj, false) end
		if obj.Hide then pcall(obj.Hide, obj) end
		if obj.SetParent then pcall(obj.SetParent, obj, nil) end
	end
	local probe = CreateFrame("Frame")
	patch(probe)
	local button = CreateFrame("Button")
	patch(button)
	local scroll = CreateFrame("ScrollFrame")
	patch(scroll)
	local edit = CreateFrame("EditBox")
	if edit.SetAutoFocus then edit:SetAutoFocus(false) end
	if edit.ClearFocus then edit:ClearFocus() end
	patch(edit)
	patch(probe:CreateTexture())
	patch(probe:CreateFontString())
	discard(edit)
	discard(scroll)
	discard(button)
	discard(probe)
end
