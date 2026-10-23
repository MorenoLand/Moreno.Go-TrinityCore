-- City/faction guards — Lua port of src/server/scripts/World/npc_guard.cpp
-- (242 lines incl. license; 2 CreatureScripts — npc_guard_generic
-- (GuardAI) + npc_guard_shattrath_faction (GuardAI); all registered
-- from inside AddSC_npc_guard() (line 237); loader decl 26 / call 47
-- per src/server/scripts/World/world_script_loader.cpp — the group
-- immediately after AddSC_go_scripts() (call 46) in AddWorldScripts()
-- — verified from the loader this run; the checkpoint sequence
-- (go_scripts -> npc_guard) is followed).
-- Sole-source verified: whole-server-tree grep for "AddSC_npc_guard"
-- hits npc_guard.cpp (+ the loader decl/call lines) only. No npc_guard
-- lua existed. Eluna creature events: 1 OnEnterCombat.
-- Entries: NPC_CENARION_HOLD_INFANTRY 15184 from the file's own
-- GuardMisc enum (the only entry with a bridgeable Talk arm);
-- NPC_STORMWIND_CITY_GUARD 68 / NPC_STORMWIND_CITY_PATROLLER 1976 /
-- NPC_ORGRIMMAR_GRUNT 3296 use npc_guard_generic's Talk-free legs;
-- NPC_ALDOR_VINDICATOR 18549 drives the Shattrath banish machine.
-- The ScriptName bindings are DB-side as usual (only entry 15184 is
-- registered below; the other entries' npc_guard_generic legs have no
-- bridgeable arms).
-- All Talk() calls in the file accounted for (1 ported below).
-- PORTED:
-- npc_guard_generic::JustEngagedWith — Talk(SAY_GUARD_SIL_AGGRO 0,
-- who) gated on me->GetEntry() == NPC_CENARION_HOLD_INFANTRY — the
-- event-1 auriaya precedent; the player target flattens through the
-- moroes precedent (Lua Talk takes no target); the _combatScheduler
-- melee/spell legs are SelectSpell machines (spell IDs unnamed in
-- C++ — picked from the creature's spell list at runtime), not named
-- timer events — no SelectSpell bridge. Registered via
-- RegisterLuaBoss("npc_guard_generic", 15184).
-- npc_guard_shattrath_faction::ScheduleVanish — the banish machine
-- (C++ npc_guard.cpp:210-231), registered for NPC_ALDOR_VINDICATOR
-- 18549 only (the one entry the C++ names; other Shattrath vindicator
-- entries bind the same ScriptName DB-side — not enumerated in-tree):
-- 5s victim-gated DoCast(SPELL_BANISHED_SHATTRATH_S 36671, default
-- args = TRIGGERED_NONE so non-triggered) + the 9s
-- ObjectAccessor::GetUnit player-GUID chain (SPELL_EXILE 39533 +
-- SPELL_BANISH_TELEPORT 36643 self-casts, explicit true =
-- TRIGGERED_FULL_MASK); re-arm is unconditional (ScheduleVanish()
-- after the 9s chain, Repeat() on the player-victim miss leg); Reset
-- cancels via the 2/4/23 maiden convention (C++ Reset/_scheduler
-- cancel + UpdateAI bails out of combat). Zero Talk in C++.
-- DOCUMENTED-ONLY:
-- npc_guard_generic::Reset — the 10-minute _scheduler buff machine
-- (SelectSpell SELECT_TARGET_ANY_FRIEND / SELECT_EFFECT_AURA self
-- buff + context.Repeat(Minutes(10))) — SelectSpell aura machine,
-- spell unnamed in C++.
-- npc_guard_generic::ReceiveEmote / DoReplyToTextEmote — the
-- Stormwind 68/1976 + Orgrimmar 3296 emote reply machine
-- (TEXT_EMOTE_KISS/WAVE/SALUTE/SHY/RUDE/CHICKEN -> EMOTE_ONESHOT_*
-- HandleEmoteCommand, IsFriendlyTo-gated) — STAYS OUT (not ported):
-- the ev8 text-emote input IS bridged (grizzly_hills lakeFrog
-- precedent; fire site engine/world/emote.go:217) and
-- creature:PerformEmote is bridged (the HandleEmoteCommand twin,
-- engine/world/lua_creature_events.go:1053) — but the
-- me->IsFriendlyTo(player) gate (C++ npc_guard.cpp:106) has NO
-- bridge: nothing on the creature event surface exposes
-- FactionTemplateEntry::IsFriendlyTo to Lua (GetFaction/SetFaction
-- are the raw field, not the relation check). Porting without the
-- gate would make guards bow/point at hostile players, so the
-- emote-reply machine stays documented-only.
-- npc_guard_generic::JustEngagedWith _combatScheduler — the 1s melee
-- machine (roll_chance_i(20) SELECT_TARGET_ANY_ENEMY DoCastVictim /
-- AttackerStateUpdate) + the 5s spell machine (HealthBelowPct(30)
-- 33%-chance SELECT_EFFECT_HEALING self-cast, else
-- SELECT_TARGET_ANY_ENEMY ranged DoCastVictim) — SelectSpell machine,
-- spell IDs unnamed in C++ (the stale "no random-target bridge"
-- parenthetical is retracted as misapplied: both machines target the
-- victim, not a random target — the SelectSpell machine and the
-- unnamed spell IDs are the blockers).
-- npc_guard_generic::UpdateAI — _scheduler.Update / UpdateVictim /
-- _combatScheduler.Update legs — no bridges.

local ENTRY_CENARION_HOLD_INFANTRY = 15184 -- NPC_CENARION_HOLD_INFANTRY

local SAY_GUARD_SIL_AGGRO = 0

-- C++ npc_guard_generic::JustEngagedWith (entry-gated in C++ on
-- NPC_CENARION_HOLD_INFANTRY): Talk(SAY_GUARD_SIL_AGGRO, who) — the
-- event-1 auriaya precedent (the target flattens per the moroes
-- precedent; the GuardAI::_combatScheduler legs have no bridges).
local function guardGenericEnterCombat(event, creature, target)
    creature:Talk(SAY_GUARD_SIL_AGGRO)
end

RegisterCreatureEvent(ENTRY_CENARION_HOLD_INFANTRY, 1, guardGenericEnterCombat)

-- npc_guard_shattrath_faction::ScheduleVanish (C++ npc_guard.cpp:210-231):
-- 5s victim-gated banish mark + 9s player-GUID exile/teleport chain.
local ENTRY_ALDOR_VINDICATOR = 18549 -- NPC_ALDOR_VINDICATOR (C++ :33)

local SPELL_BANISHED_SHATTRATH_S = 36671 -- entry == 18549 (C++ :217)
local SPELL_BANISHED_SHATTRATH_A = 36642 -- other vindicators (C++ :217)
local SPELL_BANISH_TELEPORT = 36643
local SPELL_EXILE = 39533

local TYPEID_PLAYER = 4

local banishTimers = {} -- creature GUID -> {outer=, inner=} (maiden_of_virtue convention)

local function cancelBanish(guid)
    local t = banishTimers[guid]
    if t then
        if t.inner then RemoveEventById(t.inner) end
        if t.outer then RemoveEventById(t.outer) end
        banishTimers[guid] = nil
    end
end

local armBanish -- forward-declared (the 9s chain re-arms it)

armBanish = function(creature, guid) -- C++ npc_guard.cpp:210 ScheduleVanish
    local t = banishTimers[guid] or {}
    banishTimers[guid] = t
    t.outer = CreateLuaEvent(function()
        t.outer = nil
        local victim = creature:GetVictim()
        if victim and victim:GetTypeId() == TYPEID_PLAYER then -- C++ :214-215
            -- C++ :217 DoCast(temp, spell) — default args, TRIGGERED_NONE,
            -- non-triggered like every other DoCast arm in this tree.
            local spell = SPELL_BANISHED_SHATTRATH_A
            if creature:GetEntry() == ENTRY_ALDOR_VINDICATOR then
                spell = SPELL_BANISHED_SHATTRATH_S
            end
            creature:CastSpell(victim, spell)
            local pGUID = victim:GetGUID()
            t.inner = CreateLuaEvent(function() -- C++ :219-227 9s chain
                t.inner = nil
                local p = GetPlayerByGUID(pGUID)
                if p then
                    -- C++ :223-224 temp->CastSpell(temp, ..., true) —
                    -- explicit TRIGGERED_FULL_MASK. But player:CastSpell
                    -- has no bridge (CastSpell is creature-only,
                    -- engine/world/lua_creature_events.go:60); the
                    -- creature casts on the player instead (caster
                    -- deviates from C++).
                    creature:CastSpell(p, SPELL_EXILE, true)
                    creature:CastSpell(p, SPELL_BANISH_TELEPORT, true)
                end
                armBanish(creature, guid) -- C++ :226 ScheduleVanish() re-arm (unconditional)
            end, 9000)
        else
            armBanish(creature, guid) -- C++ :230-231 banishContext.Repeat() (5s)
        end
    end, 5000)
end

local function shattrathEnterCombat(event, creature, target) -- C++ :197-199
    local guid = creature:GetGUID()
    cancelBanish(guid)
    armBanish(creature, guid)
end

local function shattrathLeaveCombat(event, creature) -- C++ Reset cancel (:193-195) + UpdateAI out-of-combat bail (:202-207)
    cancelBanish(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ALDOR_VINDICATOR, 1, shattrathEnterCombat)
RegisterCreatureEvent(ENTRY_ALDOR_VINDICATOR, 2, shattrathLeaveCombat)
RegisterCreatureEvent(ENTRY_ALDOR_VINDICATOR, 4, shattrathLeaveCombat)
RegisterCreatureEvent(ENTRY_ALDOR_VINDICATOR, 23, shattrathLeaveCombat)
