-- VX-001 (Ulduar, Mimiron phase 2) -- Lua port of two engine-fired arms of
-- boss_vx_001AI in src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp.
-- Entry (ulduar.h): NPC_VX_001 = 33651, NPC_BURST_TARGET = 34211.
-- Only the arms below are ported: every timer, DamageTaken, and DoAction
-- arm of this class is STAY-OUT (no DoAction / vehicle-kit / MovePoint /
-- GetStandState bridge; see audit). These two arms are dormant until the
-- EVENT_RAPID_BURST timer lands, since burst targets are created only by it.
--
-- PORTED (tick 195, RegisterAuraEvent):
--   spell_mimiron_rapid_burst (63382) HandleDummyTick
--   (boss_mimiron.cpp:2212-2217): EFFECT_1 PERIODIC_DUMMY; the caster
--   (VX-001) casts RAPID_BURST_LEFT 63387 on the aura target on odd
--   ticks and RAPID_BURST_RIGHT 64019 on even ticks, triggered by the
--   aura effect. No PreventDefaultAction in C++, so no return here.
--   The AfterRemove arm (:2207-2210, target TempSummon DespawnOrUnsummon)
--   stays out: no aura apply/remove hook exists on the Lua surface.

local NPC_VX_001 = 33651
local NPC_BURST_TARGET = 34211

local SPELL_RAPID_BURST = 63382
local SPELL_RAPID_BURST_LEFT = 63387
local SPELL_RAPID_BURST_RIGHT = 64019
local SPELL_RAPID_BURST_TARGET_ME = 64841

-- C++ JustSummoned (1022-1027): a summoned burst target casts
-- RAPID_BURST_TARGET_ME on VX-001. Event 19 args: (event, creature, summoned);
-- the summoned object is a full handler creature, so it can cast.
local function vx001JustSummoned(event, creature, summoned)
    if summoned ~= nil and summoned:GetEntry() == NPC_BURST_TARGET then
        summoned:CastSpell(creature, SPELL_RAPID_BURST_TARGET_ME)
    end
end

-- C++ SpellHit (1036-1040): if the hitting caster is a burst target and
-- VX-001 is not casting, VX-001 casts RAPID_BURST on it. Event 14 args:
-- (event, creature, caster, spellId); GetEntry resolves via the generic
-- object fallback. IsCasting is the constant-false twin on the handler
-- surface (caststate axiom), so the gate is vacuous in Go -- kept for
-- C++-exactness, matching the halazzi precedent.
local function vx001SpellHit(event, creature, caster, spellId)
    if caster ~= nil and caster:GetEntry() == NPC_BURST_TARGET and not creature:IsCasting() then
        creature:CastSpell(caster, SPELL_RAPID_BURST)
    end
end

local function rapidBurstTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        local spell = SPELL_RAPID_BURST_LEFT
        if aura:GetTickNumber() % 2 == 0 then
            spell = SPELL_RAPID_BURST_RIGHT
        end
        caster:CastSpell(target, spell, true)
    end
end

RegisterCreatureEvent(NPC_VX_001, 19, vx001JustSummoned)
RegisterCreatureEvent(NPC_VX_001, 14, vx001SpellHit)
RegisterAuraEvent(SPELL_RAPID_BURST, 1, rapidBurstTick)
