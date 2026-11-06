-- Death Knight spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_dk.cpp.
--
-- PORTED (tick 294, RegisterAuraEvent):
--   spell_dk_presence (spell_dk.cpp:1859-1989): the three presence auras
--   48266 (Blood), 48263 (Frost), 48265 (Unholy). One mode-2 handler per
--   ID runs all three legs, branching on aura:GetAuraId(), because C++
--   registers all three AfterEffectApply handlers on each presence
--   (AURA_EFFECT_HANDLE_REAL, EFFECT_0, SPELL_AURA_ANY).
--   Blood leg: id 48266 casts 63611 triggered; else the Improved Blood
--   Presence effect (GetAuraEffectOfRankedSpell(50365, 0)) casts 63611
--   with BP1 = that amount, unless 63611 is already present.
--   Frost leg: id 48263 casts 61261 triggered; else the Improved Frost
--   Presence effect (50384, 0) casts 61261 with BP0 = that amount,
--   unless 61261 is already present.
--   Unholy leg: id 48265 casts 49772 triggered; the Improved Unholy
--   Presence effect (50391, 0), when present, casts 63622 with all
--   three BPs = the EFFECT_1 CalcValue of spell 50391 (DBC read, 0
--   skips the cast), or casts 49772 with BP0 = that amount when a
--   different presence is up and 49772 is absent.
--   Mode 3 (all three IDs): unconditionally removes 63611, 63622,
--   61261 and 49772.
--   spell_dk_improved_blood_presence (spell_dk.cpp:1442-1493): aura
--   50365 (rank 1 only; ranks 2-3 IDs are not in the file, never
--   guessed). Mode 2: while Frost (48263) or Unholy (48265) presence
--   is up and 63611 is absent, casts 63611 with BP1 = this aura's
--   EFFECT_0 amount. Mode 3: removes 63611 unless Blood presence
--   (48266) is up.
--   spell_dk_improved_frost_presence (spell_dk.cpp:1541-1592): aura
--   50384 (rank 1 only). Mode 2: while Blood (48266) or Unholy
--   (48265) presence is up and 61261 is absent, casts 61261 with BP0
--   = this aura's EFFECT_0 amount. Mode 3: removes 61261 unless
--   Frost presence (48263) is up.
--   spell_dk_improved_unholy_presence (spell_dk.cpp:1593-1658):
--   aura 50391 (rank 1 only). Mode 2 leg A: while Unholy presence
--   (48265) is up and 63622 is absent, casts 63622 with all three
--   BPs = the EFFECT_1 CalcValue of spell 50391 (DBC read, 0 skips
--   the cast). Leg B: while Blood (48266) or Frost (48263) presence
--   is up and 49772 is absent, casts 49772 with BP0 = this aura's
--   EFFECT_0 amount. Mode 3: removes 63622 unconditionally and
--   removes 49772 unless Unholy presence (48265) is up.
--   CastCustomSpell nil BP args skip that effect (luaCastCustomBasePoints),
--   so the BP1-only 63611 cast passes nil for BP0.
--   Mode 2 fires on fresh apply only; the C++ legs are all
--   AURA_EFFECT_HANDLE_REAL, so this matches C++ (no reapply leg).
-- STAY-OUT (tick 294): every other spell_dk class — no proc hook
--   (OnEffectProc/CheckProc), absorb/CalcAmount/ManaShield hooks
--   absent, or SpellScript-only with no RegisterSpellScript in the
--   engine. spell_dk_death_and_decay is entry-blocked: its aura
--   spell ID lives in DB spell_script_names, nowhere in the file.
--   spell_dk_blood_tap needs the rune system; spell_dk_hysteria
--   needs DealDamage; spell_dk_raise_ally needs ghoul AI push/pop.

local SPELL_DK_BLOOD_PRESENCE = 48266
local SPELL_DK_FROST_PRESENCE = 48263
local SPELL_DK_UNHOLY_PRESENCE = 48265
local SPELL_DK_IMPROVED_BLOOD_PRESENCE_R1 = 50365
local SPELL_DK_IMPROVED_FROST_PRESENCE_R1 = 50384
local SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1 = 50391
local SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED = 63611
local SPELL_DK_FROST_PRESENCE_TRIGGERED = 61261
local SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED = 63622
local SPELL_DK_UNHOLY_PRESENCE_TRIGGERED = 49772

local function presenceApply(event, aura, target)
    local id = aura:GetAuraId()
    if id == SPELL_DK_BLOOD_PRESENCE then
        target:CastSpell(target, SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED, true)
    else
        local imp = target:GetAuraEffectOfRankedSpell(SPELL_DK_IMPROVED_BLOOD_PRESENCE_R1, 0)
        if imp and not target:HasAura(SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED) then
            target:CastCustomSpell(target, SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED, true, nil, imp)
        end
    end
    if id == SPELL_DK_FROST_PRESENCE then
        target:CastSpell(target, SPELL_DK_FROST_PRESENCE_TRIGGERED, true)
    else
        local imp = target:GetAuraEffectOfRankedSpell(SPELL_DK_IMPROVED_FROST_PRESENCE_R1, 0)
        if imp and not target:HasAura(SPELL_DK_FROST_PRESENCE_TRIGGERED) then
            target:CastCustomSpell(target, SPELL_DK_FROST_PRESENCE_TRIGGERED, true, imp)
        end
    end
    if id == SPELL_DK_UNHOLY_PRESENCE then
        target:CastSpell(target, SPELL_DK_UNHOLY_PRESENCE_TRIGGERED, true)
    end
    local imp = target:GetAuraEffectOfRankedSpell(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 0)
    if imp then
        if id == SPELL_DK_UNHOLY_PRESENCE then
            local bp = aura:GetSpellEffectCalcValue(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 1)
            if bp ~= 0 then
                target:CastCustomSpell(target, SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED, true, bp, bp, bp)
            end
        elseif not target:HasAura(SPELL_DK_UNHOLY_PRESENCE_TRIGGERED) then
            target:CastCustomSpell(target, SPELL_DK_UNHOLY_PRESENCE_TRIGGERED, true, imp)
        end
    end
end

local function presenceRemove(event, aura, target)
    target:RemoveAura(SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED)
    target:RemoveAura(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED)
    target:RemoveAura(SPELL_DK_FROST_PRESENCE_TRIGGERED)
    target:RemoveAura(SPELL_DK_UNHOLY_PRESENCE_TRIGGERED)
end

for _, spellId in ipairs({ SPELL_DK_BLOOD_PRESENCE, SPELL_DK_FROST_PRESENCE, SPELL_DK_UNHOLY_PRESENCE }) do
    RegisterAuraEvent(spellId, 2, presenceApply)
    RegisterAuraEvent(spellId, 3, presenceRemove)
end

local function improvedBloodPresenceApply(event, aura, target)
    if (target:HasAura(SPELL_DK_FROST_PRESENCE) or target:HasAura(SPELL_DK_UNHOLY_PRESENCE))
        and not target:HasAura(SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED) then
        target:CastCustomSpell(target, SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED, true, nil, aura:GetAmount())
    end
end

local function improvedBloodPresenceRemove(event, aura, target)
    if not target:HasAura(SPELL_DK_BLOOD_PRESENCE) then
        target:RemoveAura(SPELL_DK_IMPROVED_BLOOD_PRESENCE_TRIGGERED)
    end
end

RegisterAuraEvent(SPELL_DK_IMPROVED_BLOOD_PRESENCE_R1, 2, improvedBloodPresenceApply)
RegisterAuraEvent(SPELL_DK_IMPROVED_BLOOD_PRESENCE_R1, 3, improvedBloodPresenceRemove)

local function improvedFrostPresenceApply(event, aura, target)
    if (target:HasAura(SPELL_DK_BLOOD_PRESENCE) or target:HasAura(SPELL_DK_UNHOLY_PRESENCE))
        and not target:HasAura(SPELL_DK_FROST_PRESENCE_TRIGGERED) then
        target:CastCustomSpell(target, SPELL_DK_FROST_PRESENCE_TRIGGERED, true, aura:GetAmount())
    end
end

local function improvedFrostPresenceRemove(event, aura, target)
    if not target:HasAura(SPELL_DK_FROST_PRESENCE) then
        target:RemoveAura(SPELL_DK_FROST_PRESENCE_TRIGGERED)
    end
end

RegisterAuraEvent(SPELL_DK_IMPROVED_FROST_PRESENCE_R1, 2, improvedFrostPresenceApply)
RegisterAuraEvent(SPELL_DK_IMPROVED_FROST_PRESENCE_R1, 3, improvedFrostPresenceRemove)

local function improvedUnholyPresenceApply(event, aura, target)
    if target:HasAura(SPELL_DK_UNHOLY_PRESENCE)
        and not target:HasAura(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED) then
        local bp = aura:GetSpellEffectCalcValue(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 1)
        if bp ~= 0 then
            target:CastCustomSpell(target, SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED, true, bp, bp, bp)
        end
    end
    if (target:HasAura(SPELL_DK_BLOOD_PRESENCE) or target:HasAura(SPELL_DK_FROST_PRESENCE))
        and not target:HasAura(SPELL_DK_UNHOLY_PRESENCE_TRIGGERED) then
        target:CastCustomSpell(target, SPELL_DK_UNHOLY_PRESENCE_TRIGGERED, true, aura:GetAmount())
    end
end

local function improvedUnholyPresenceRemove(event, aura, target)
    target:RemoveAura(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_TRIGGERED)
    if not target:HasAura(SPELL_DK_UNHOLY_PRESENCE) then
        target:RemoveAura(SPELL_DK_UNHOLY_PRESENCE_TRIGGERED)
    end
end

RegisterAuraEvent(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 2, improvedUnholyPresenceApply)
RegisterAuraEvent(SPELL_DK_IMPROVED_UNHOLY_PRESENCE_R1, 3, improvedUnholyPresenceRemove)
