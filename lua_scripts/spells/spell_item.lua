-- Item spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_item.cpp.
--
-- PORTED (tick 199, RegisterAuraEvent OnApply = 2, OnPeriodic = 1,
-- OnRemove = 3):
--   spell_item_nitro_boosts_backfire::HandleApply
--   (spell_item.cpp:3107-3110): EFFECT_1 SPELL_AURA_PERIODIC_TRIGGER_SPELL.
--   On apply, records the target's height; the C++ member lastZ
--   (INVALID_HEIGHT init) is the per-target table below, keyed by
--   the target GUID — the apply hook always precedes the periodic
--   hook, so the entry exists by the first tick.
--   spell_item_nitro_boosts_backfire::HandlePeriodicDummy
--   (spell_item.cpp:3112-3123): PreventDefaultAction, so the handler
--   returns true and the Go default trigger cast does not fire. A
--   falling target (current Z below lastZ) rolls 80% to self-cast
--   the parachute 54649, triggered (CastSpellExtraArgs(AuraEffect*)
--   sets TRIGGERED_FULL_MASK, SpellDefines.h:166), then the aura
--   removes itself; a rising target just updates lastZ. The remove
--   handler drops the table entry.

local SPELL_NITRO_BOOSTS_BACKFIRE = 54621 -- spell_item.cpp:3098 (class spell_item_nitro_boosts_backfire)
local SPELL_NITRO_BOOSTS_PARACHUTE = 54649 -- spell_item.cpp:3063

local lastZByGUID = {}

local function nitroBackfireApply(event, aura, target)
    lastZByGUID[target:GetGUID()] = target:GetZ()
end

local function nitroBackfirePeriodic(event, aura, target)
    local guid = target:GetGUID()
    local lastZ = lastZByGUID[guid]
    local curZ = target:GetZ()
    if lastZ == nil then
        lastZByGUID[guid] = curZ
        return true
    end
    if curZ < lastZ then
        if math.random(0, 99) < 80 then
            target:CastSpell(target, SPELL_NITRO_BOOSTS_PARACHUTE, true)
        end
        aura:Remove()
        lastZByGUID[guid] = nil
    else
        lastZByGUID[guid] = curZ
    end
    return true
end

local function nitroBackfireRemove(event, aura, target)
    lastZByGUID[target:GetGUID()] = nil
end

RegisterAuraEvent(SPELL_NITRO_BOOSTS_BACKFIRE, 2, nitroBackfireApply)
RegisterAuraEvent(SPELL_NITRO_BOOSTS_BACKFIRE, 1, nitroBackfirePeriodic)
RegisterAuraEvent(SPELL_NITRO_BOOSTS_BACKFIRE, 3, nitroBackfireRemove)
