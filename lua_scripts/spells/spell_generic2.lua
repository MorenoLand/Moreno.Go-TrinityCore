-- Generic spell scripts, part 2 — Lua port of
-- src/server/scripts/Spells/spell_generic.cpp (continued from
-- spell_generic.lua, which hit the go-lua main-chunk ceiling of 200
-- top-level locals; new spell_generic classes land here instead, one
-- table local per strip, so the old file stays loadable).

-- C++ spell_gen_turkey_marker::OnApply + OnPeriodic
-- (spell_generic.cpp:3392-3439): 25281 EFFECT_0 SPELL_AURA_PERIODIC_DUMMY.
-- OnApply (REAL_OR_REAPPLY) records the stack-apply time and, at >= 15
-- stacks, the target casts Turkey Vengeance 25285 with
-- CastSpellExtraArgs(aurEff, casterGUID) — TRIGGERED_FULL_MASK
-- (SpellDefines.h:168) — so the Lua cast is triggered. OnPeriodic pops
-- apply-times older than the aura max duration and ModStackAmount(-n,
-- AURA_REMOVE_BY_EXPIRE).
-- Go shape: ev2 OnApply fires on fresh applies only, so the stack-raise
-- leg rides ev4 OnChangeAmount (fires on merge re-casts that raise the
-- stack, spells.go player/creature merge legs); one shared handler
-- covers both. Deltas: (1) a duration-only refresh fires neither ev2
-- nor ev4, so it records no apply-time and the stack later sheds one
-- fewer expired stack per refresh than C++; (2) Go SetStackAmount does
-- not remove at 0 (unlike C++ ModStackAmount to zero), so the handler
-- calls aura:Remove() explicitly at zero; (3) the per-aura apply-time
-- list rides a Lua-side table keyed by target GUID (in-file precedent:
-- boss_netherspite.lua stores target GUIDs; aura ScriptValues hold one
-- int64, not a list) and is cleared by the ev3 bookkeeping handler —
-- that ev3 leg is not C++ behaviour, only table hygiene; (4) a missing
-- DBC duration reads GetMaxDuration() -1/0, and the periodic leg
-- no-ops instead of wiping the stack. DORMANT until Spell.dbc ships
-- the 25281 row (not in tools/spelldbc_seed/main.go; seed axis parked).
-- C++ spell_gen_lifebloom::AfterRemove (spell_generic.cpp:1987-2016):
-- EFFECT_0 SPELL_AURA_PERIODIC_HEAL, AURA_EFFECT_HANDLE_REAL. The final
-- heal fires only when the remove mode is AURA_REMOVE_BY_EXPIRE or
-- AURA_REMOVE_BY_ENEMY_SPELL; the target casts the mapped final-heal
-- spell with CastSpellExtraArgs(aurEff, casterGUID) — TRIGGERED_FULL_MASK
-- (SpellDefines.h:168). CORRECTION of the 2026-11-19 triage C16 row:
-- the C++ passes NO AddSpellBP0, so the final heal uses its own DBC
-- EFFECT_0 base points — a CastCustomSpell BP0 of aura:GetAmount()
-- would wrongly override the DBC heal with the periodic tick amount.
-- The port is therefore a plain triggered CastSpell. Remaining delta:
-- the original-caster attribution (GetCasterGUID) has no Lua twin
-- (CastCustomSpell accepts an originalCaster arg but the funnel leaves
-- it unmodeled, lua_player.go:3820-3827), so the heal is cast by the
-- target rather than attributed to the original caster. Aura -> final
-- heal map from the C++ WithArgs registrations (:4401-4405) joined to
-- the TDB/AC spell_script_names rows: 43421->43422 (hexlord),
-- 52551->52552 (tur_ragepaw), 53608->53692 (cenarion_scout),
-- 57762/59990->57763 (twisted_visage), 66093/67957/67958/67959->66094
-- (faction_champion_dru). -33763 spell_dru_lifebloom is the druid
-- class spell, a different script, not this class. DORMANT until
-- Spell.dbc ships the aura rows (none in tools/spelldbc_seed/main.go).
-- C++ spell_gen_moss_covered_feet::HandleProc (spell_generic.cpp:2323-2342):
-- 6870 / 31399 EFFECT_0 SPELL_AURA_DUMMY. PreventDefaultAction; the
-- proc action target casts Fall Down 6869 on nil with the triggering
-- aurEff. CORRECTION of the 2026-11-19 triage C19 row: the third
-- CastSpell arg is the aurEff, i.e. CastSpellExtraArgs(aurEff) ->
-- TRIGGERED_FULL_MASK (SpellDefines.h:166), so the cast IS triggered,
-- not non-triggered as the triage claimed. The aurEff attribution
-- itself has no Lua twin (plain triggered CastSpell). DORMANT until
-- Spell.dbc ships the 6870/31399 rows (not in tools/spelldbc_seed/main.go).
local SPELL_GEN_STRIP13 = {
    turkeyMarker = 25281,
    turkeyVengeance = 25285,
    turkeyVengeanceStacks = 15,
    turkeyApplyTimes = {},
    lifebloomFinalHeal = {
        [43421] = 43422,
        [52551] = 52552,
        [53608] = 53692,
        [57762] = 57763,
        [59990] = 57763,
        [66093] = 66094,
        [67957] = 66094,
        [67958] = 66094,
        [67959] = 66094,
    },
    mossCoveredFeet = { 6870, 31399 },
    fallDown = 6869,
    removeByEnemySpell = 3, -- spells.go:36611-36616 iota (None 0, Default 1, Cancel 2)
    removeByExpire = 4, -- spells.go:36615; C++ AURA_REMOVE_BY_EXPIRE is 5, Go is 4

    turkeyStackApplied = function(event, aura, target)
        if target == nil then
            return
        end
        local guid = target:GetGUID()
        local times = SPELL_GEN_STRIP13.turkeyApplyTimes[guid]
        if times == nil then
            times = {}
            SPELL_GEN_STRIP13.turkeyApplyTimes[guid] = times
        end
        times[#times + 1] = GetGameTimeMS()
        if aura:GetStackAmount() >= SPELL_GEN_STRIP13.turkeyVengeanceStacks then
            target:CastSpell(target, SPELL_GEN_STRIP13.turkeyVengeance, true)
        end
    end,

    turkeyPeriodic = function(event, aura, target)
        if target == nil then
            return
        end
        local maxDuration = aura:GetMaxDuration()
        if maxDuration <= 0 then
            return
        end
        local times = SPELL_GEN_STRIP13.turkeyApplyTimes[target:GetGUID()]
        if times == nil then
            return
        end
        local now = GetGameTimeMS()
        local removeCount = 0
        while #times > 0 and times[1] + maxDuration < now do
            table.remove(times, 1)
            removeCount = removeCount + 1
        end
        if removeCount > 0 then
            local newStacks = aura:GetStackAmount() - removeCount
            if newStacks <= 0 then
                aura:Remove()
            else
                aura:SetStackAmount(newStacks)
            end
        end
    end,

    turkeyRemoved = function(event, aura, target)
        if target == nil then
            return
        end
        SPELL_GEN_STRIP13.turkeyApplyTimes[target:GetGUID()] = nil
    end,

    lifebloomRemoved = function(event, aura, target)
        local mode = aura:GetRemoveMode()
        if mode ~= SPELL_GEN_STRIP13.removeByExpire and mode ~= SPELL_GEN_STRIP13.removeByEnemySpell then
            return
        end
        if target == nil then
            return
        end
        local healId = SPELL_GEN_STRIP13.lifebloomFinalHeal[aura:GetAuraId()]
        if healId == nil then
            return
        end
        target:CastSpell(target, healId, true)
    end,

    mossCoveredFeetProc = function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
        if effectIndex ~= 0 or procEvent == nil then
            return true
        end
        local actionTarget = procEvent:GetActionTarget()
        if actionTarget == nil then
            return true
        end
        actionTarget:CastSpell(nil, SPELL_GEN_STRIP13.fallDown, true)
        return true
    end,
}

RegisterAuraEvent(SPELL_GEN_STRIP13.turkeyMarker, 2, SPELL_GEN_STRIP13.turkeyStackApplied)
RegisterAuraEvent(SPELL_GEN_STRIP13.turkeyMarker, 4, SPELL_GEN_STRIP13.turkeyStackApplied)
RegisterAuraEvent(SPELL_GEN_STRIP13.turkeyMarker, 1, SPELL_GEN_STRIP13.turkeyPeriodic)
RegisterAuraEvent(SPELL_GEN_STRIP13.turkeyMarker, 3, SPELL_GEN_STRIP13.turkeyRemoved)
for auraId, healId in pairs(SPELL_GEN_STRIP13.lifebloomFinalHeal) do
    RegisterAuraEvent(auraId, 3, SPELL_GEN_STRIP13.lifebloomRemoved)
end
for _, auraId in ipairs(SPELL_GEN_STRIP13.mossCoveredFeet) do
    RegisterAuraEvent(auraId, 6, SPELL_GEN_STRIP13.mossCoveredFeetProc)
end

-- C++ spell_gen_summon_elemental::AfterApply + AfterRemove
-- (spell_generic.cpp:3156-3197): 40132 earth / 40133 fire,
-- EFFECT_1 SPELL_AURA_DUMMY REAL. AfterApply: the aura caster's
-- owner casts the summoned elemental (earth 19704, fire 8985)
-- triggered on itself. AfterRemove: the owner player RemovePet —
-- STAYS OUT, no RemovePet bridge anywhere in engine/ (grep
-- zero hits; the only RemovePet matches are comments and
-- RemovePetitionsAndSigns). Apply leg only. Registration map
-- from the AC spell_script_names rows (TDB 26091 has no rows
-- for 40132/40133): 40132 -> spell_gen_summon_earth_elemental,
-- 40133 -> spell_gen_summon_fire_elemental. Bridges: aura
-- GetCaster (lua_aura.go:339, resolves via
-- luaUnitObjectForGUID); creature-surface GetOwner
-- (lua_creature_events.go:702) returns the owning player
-- session object; player CastSpell (lua_player.go:3805).
-- C++ registers AfterEffectApply with AURA_EFFECT_HANDLE_REAL
-- (fresh applies only), matching Go ev2. DORMANT until
-- Spell.dbc ships the 40132/40133 rows (not in
-- tools/spelldbc_seed/main.go; seed axis parked).
-- C++ spell_gen_animal_blood::OnRemove
-- (spell_generic.cpp:175-204): 46221, EFFECT_0
-- PERIODIC_TRIGGER_SPELL REAL. On remove the aura owner
-- casts Spawn Blood Pool 63471 triggered on itself (ev3).
-- The OnApply dedupe leg STAYS OUT: it enumerates the
-- owner's auras via GetOwnedAura/RemoveOwnedAura, and
-- neither has a Lua bridge — without it, duplicate 46221
-- auras (e.g. from different casters) each spawn a pool on
-- remove, where C++ keeps only one. TDB 26091 + AC bind
-- 46221 to spell_gen_animal_blood. DORMANT until Spell.dbc
-- ships the 46221 row (not in tools/spelldbc_seed/main.go).
local SPELL_GEN_STRIP14 = {
    summonElemental = {
        [40132] = 19704, -- earth, spell_generic.cpp:4446
        [40133] = 8985, -- fire, spell_generic.cpp:4447
    },
    animalBlood = 46221, -- spell_generic.cpp:175
    spawnBloodPool = 63471, -- spell_generic.cpp:170

    summonElementalApply = function(event, aura, target)
        local summonedId = SPELL_GEN_STRIP14.summonElemental[aura:GetAuraId()]
        if summonedId == nil then
            return
        end
        local caster = aura:GetCaster()
        if caster == nil then
            return
        end
        local owner = caster:GetOwner()
        if owner == nil then
            return
        end
        owner:CastSpell(owner, summonedId, true)
    end,

    animalBloodRemoved = function(event, aura, target)
        if target == nil then
            return
        end
        target:CastSpell(target, SPELL_GEN_STRIP14.spawnBloodPool, true)
    end,
}

for auraId in pairs(SPELL_GEN_STRIP14.summonElemental) do
    RegisterAuraEvent(auraId, 2, SPELL_GEN_STRIP14.summonElementalApply)
end
RegisterAuraEvent(SPELL_GEN_STRIP14.animalBlood, 3, SPELL_GEN_STRIP14.animalBloodRemoved)

-- C++ spell_gen_decay_over_time_spell::ModAuraStack
-- (spell_generic.cpp:1324-1338): AfterHit. The hit aura's stack is
-- set to SpellInfo::StackAmount. The aura is applied earlier in the
-- same cast (spells.go applyAuraToTarget runs before the AfterHit
-- fire on both paths), so the GetHitAura twin reads it: the hit
-- target's aura of this spell at any rank, guarded on this cast's
-- caster (the Haunt idiom, spell_warlock.lua) — C++ GetHitAura is
-- m_spell->_spellAura (Spell.cpp:2908), the aura this cast
-- created/refreshed on the hit target. The StackAmount value rides
-- the new spell:GetStackAmount bridge (DBC field 49), so no literal
-- is guessed; a missing DBC row reads 0 and the handler no-ops.
-- CORRECTION of the head NEXT + the spell_generic.lua:1026/:1545
-- notes: no new GetHitAura bridge is needed — the Haunt twin above
-- IS the GetHitAura mechanism, already bridged on both unit surfaces
-- (lua_player.go:3359 + lua_creature_events.go:1996,
-- GetAuraOfRankedSpell). DELTA: 36659 Tail Sting is a pet-cast
-- ability and the pet path fires no spell hooks
-- (executePetSpellWithOptions has no fireSpellAfterHit site), so its
-- spell half cannot fire until the pet path gains hook fires; the
-- registration is path-agnostic and fires on any player/creature
-- cast. DORMANT until Spell.dbc ships the 32065/36659 rows (not in
-- tools/spelldbc_seed/main.go; seed axis parked).
-- C++ spell_gen_landmine_knockback_achievement::HandleScript
-- (spell_generic.cpp:4030-4051): 57099 EFFECT_0 SPELL_EFFECT_SCRIPT_
-- EFFECT. OnEffectHitTarget, when the hit player carries the hit
-- aura at >= 10 stacks, the player casts the achievement spell
-- 57064 triggered on itself. Same GetHitAura twin as above, plus the
-- player gate (GetHitPlayer, TYPEID_PLAYER = 4). ORDERING DELTA: Go
-- fires ev5 before the default apply arms on both paths (spells.go
-- ev5 loop ahead of the apply arm; boss_ai.go:3562 ahead of :3814),
-- while C++ reads the post-application stack — this cast already
-- merged +1 into it (TryRefreshStackOrCreate, Unit.cpp:3326 twin in
-- the Go merge leg). The port therefore gates on the pre-cast stack
-- >= 9, i.e. (pre-cast + 1) >= 10, AND on spell:GetStackAmount() >=
-- 10, so a spell whose ceiling is below 10 can never fire early the
-- way a bare >= 9 gate would. No cast site for 57099 exists in the
-- C++ tree outside this registration; the registration fires on
-- whichever path casts it. TDB 26091 binds 57099 to
-- spell_gen_landmine_knockback_achievement (AC says
-- spell_gen_mine_sweeper, divergent name, same id). DORMANT until
-- Spell.dbc ships the 57099 row.
local SPELL_GEN_STRIP15 = {
    decayOverTime = { 32065, 36659 }, -- spell_generic.cpp:4381-4382
    landmine = 57099, -- spell_generic.cpp:4030
    landmineAchievement = 57064, -- spell_generic.cpp:4027
    landmineStacks = 10, -- spell_generic.cpp:4040
    typeIdPlayer = 4,

    decayAfterHit = function(event, spell, hitTarget)
        if hitTarget == nil then
            return
        end
        local aura = hitTarget:GetAuraOfRankedSpell(spell:GetEntry())
        if aura == nil then
            return
        end
        local caster = spell:GetCaster()
        if caster ~= nil and aura:GetCasterGUID() ~= caster:GetGUID() then
            return
        end
        local stackAmount = spell:GetStackAmount()
        if stackAmount <= 0 then
            return
        end
        aura:SetStackAmount(stackAmount)
    end,

    landmineHit = function(event, spell, hitTarget, effectIndex)
        if effectIndex ~= 0 then
            return
        end
        if hitTarget == nil or hitTarget:GetTypeId() ~= SPELL_GEN_STRIP15.typeIdPlayer then
            return
        end
        local aura = hitTarget:GetAuraOfRankedSpell(SPELL_GEN_STRIP15.landmine)
        if aura == nil then
            return
        end
        local caster = spell:GetCaster()
        if caster ~= nil and aura:GetCasterGUID() ~= caster:GetGUID() then
            return
        end
        if spell:GetStackAmount() < SPELL_GEN_STRIP15.landmineStacks then
            return
        end
        if aura:GetStackAmount() + 1 < SPELL_GEN_STRIP15.landmineStacks then
            return
        end
        hitTarget:CastSpell(hitTarget, SPELL_GEN_STRIP15.landmineAchievement, true)
    end,
}

for _, spellId in ipairs(SPELL_GEN_STRIP15.decayOverTime) do
    RegisterSpellEvent(spellId, 3, SPELL_GEN_STRIP15.decayAfterHit)
end
RegisterSpellEvent(SPELL_GEN_STRIP15.landmine, 5, SPELL_GEN_STRIP15.landmineHit)
