-- Grizzly Hills: npc_lake_frog — Lua port of
-- src/server/scripts/Northrend/zone_grizzly_hills.cpp
-- (npc_lake_frog : public CreatureScript; registered in AddSC_zone_grizzly_hills()).
-- Entries: NPC_LAKE_FROG = 33211 / NPC_LAKE_FROG_QUEST = 33224
-- (zone_grizzly_hills.cpp:655-667 LakeFrog enum). The C++ tree has a single
-- npc_lake_frog script (one `new` at :1130), so both entries share this AI —
-- both are registered.
-- Ported: Reset's RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) twin on
-- spawn (event 5) — C++ Reset (:702-707) is entry-conditional: it removes
-- the flag ONLY for NPC_LAKE_FROG_QUEST (33224), so only that entry is
-- registered below. 33211 keeps its template flag exactly like C++.
-- The select arm below stays gated exactly like C++ —
-- plus the OnGossipSelect DoCast(player, SPELL_SUMMON_ASHWOOD_BRAND 62554)
-- arm (:774-778), non-triggered (default DoCast overload); return false lets
-- the AI/native select arms proceed (Eluna gossip polarity, gossip.go).
-- Ported: the ReceiveEmote kiss machine (zone_grizzly_hills.cpp:746-777,
-- npc_lake_frogAI::ReceiveEmote, ev8 — the trigger is the emote, not
-- MoveInLineOfSight): TEXT_EMOTE_KISS (58) within 30yd while the player
-- carries the Warts-B-Gone Lip Balm (44986). player:HasItem,
-- creature:IsWithinDist (object.go generic fallback) and every aura leg are
-- bridged. Without the lip-balm aura (62574) the player gets SPELL_WARTS
-- (62581); with it, the frog casts SPELL_FROG_KISS (62536, non-triggered
-- DoCast twin) and 33211 takes SPELL_FROG_LOVE (62537). The
-- _following/_runningScript re-entry latch is a per-GUID table, and the
-- UpdateAI followup (:699-701 — the follower despawns once FROG_LOVE fades;
-- the aura lasts 60s) rides a 61s one-shot CreateLuaEvent, rechecked until
-- the aura is gone (C++ polls it every UpdateAI tick).
-- Ported 2026-10-24: the 33224 quest-frog branch — SetFacingToObject
-- via the atan2 + SetFacingTo twin + the EVENT_LAKEFROG_1..5
-- maiden-transform chain (DoCast 62550, UpdateEntry 33220, maiden
-- Talk SAY_MAIDEN_0/1, gossip-flag windows, despawn 4s after the last
-- Talk); the old "no bridges" line was stale — SetFacingTo,
-- UpdateEntry, SetFlag/RemoveFlag and Talk are all live on the
-- creature surface. Stays out: MoveFollow (no movement bridge) and
-- MoveIdle (vacuous — the frog stands still).
-- npc_venture_co_straggler (zone_grizzly_hills.cpp:562-651) — NOT
-- registered: no entry constant anywhere in the C++ tree (DB ScriptName
-- binding only, unverifiable — wounded-skirmisher precedent), so no
-- entry-verifiable registration can be wired. The arms themselves are
-- mostly bridgeable now (Reset's RemoveFlag NON_ATTACKABLE +
-- SetImmuneToPC(false) (= RemoveFlag(59, 0x100)) + SetReactState
-- (REACT_AGGRESSIVE); the SpellHit SMOKE_BOMB 49075 arm rides event 14
-- (live fire sites: boss_ai.go:2125/2134, spells.go:11765) with the
-- caster available for the _playerGUID credit leg; SetFlag
-- NON_ATTACKABLE + SetFlag(59, 0x100) + SetReactState(REACT_PASSIVE) +
-- Talk(SAY_SEO) + the EVENT_STRAGGLER_1..4 CreateLuaEvent chain) — but
-- CombatStop is bridged (2026-10-24) but the three MovePoint legs +
-- DisappearAndDie have no bridges, and without an entry the separable CHOP/Reset arms can't be
-- registered either. Revisit only if an entry becomes C++-verifiable.
-- Documented-only (nearest-creature axis): npc_amberpine_woodsman
-- (zone_grizzly_hills.cpp:409-457): Reset's
-- FindNearestCreature(NPC_TALLHORN_STAG 26363, 0.2f) emote-state gate
-- (:420 — EMOTE_STATE_USE_STANDING vs. the EVENT_WOODSMAN_1/2
-- loot/attack emote loop) has a live bridge twin, but the AI is not
-- registered in boss_ai.go and carries no NPC_ entry constant anywhere
-- in the C++ tree (DB ScriptName binding only, unverifiable —
-- wounded-skirmisher precedent); stays out as unregistered AI.
-- Ported (find-nearest-gameobject axis): npc_tallhorn_stag
-- (zone_grizzly_hills.cpp:339-390 — npc_tallhorn_stag : public
-- CreatureScript, registered in AddSC_zone_grizzly_hills()). Entry
-- NPC_TALLHORN_STAG = 26363 comes from the C++ AmberpineWoodsman enum
-- (zone_grizzly_hills.cpp:403-405), so the registration is C++-verifiable.
-- Ported: the Reset/UpdateAI _phase arm (:373-388) — the first UpdateAI tick
-- after Reset runs it exactly once (_phase 1 -> 0). It checks for the haunch
-- GO (OBJECT_HAUNCH = 188665, 2.0yd); when found the stag plays dead:
-- SetStandState(UNIT_STAND_STATE_DEAD = 7), SetImmuneToPC(true) (bridged as
-- creature:SetFlag(59, 0x100) — UNIT_FLAG_IMMUNE_TO_PC; the engine honors
-- 0x100 in the PC-targeting gate, creaturemotion.go:4213), and
-- SetUInt32Value(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_DEAD) (bridged as
-- creature:SetFlag(79, 0x20) — UNIT_DYNFLAG_DEAD is 0x20, kill.go:45; C++
-- sets the raw field while the twin ORs the bit, identical on the
-- zero-initialized spawn). The GO layout is static between Reset and the
-- first AI tick, so running the check synchronously on OnSpawn(5) /
-- OnReset(23) is C++-exact; the arm is idempotent, so both Reset call sites
-- are safe. The UpdateVictim/DoMeleeAttackIfReady tail is default engine
-- behavior — no port needed.
local ENTRY_LAKE_FROG = 33211
local ENTRY_LAKE_FROG_QUEST = 33224
local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_GOSSIP = 1
local SPELL_SUMMON_ASHWOOD_BRAND = 62554
local ENTRY_TALLHORN_STAG = 26363
local OBJECT_HAUNCH = 188665
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local UNIT_DYNAMIC_FLAGS_FIELD = 79
local UNIT_DYNFLAG_DEAD = 0x20
local UNIT_STAND_STATE_DEAD = 7

local ITEM_WARTS_B_GONE_LIP_BALM = 44986
local SPELL_WARTSBGONE_LIP_BALM = 62574
local SPELL_FROG_LOVE = 62537
local SPELL_WARTS = 62581
local SPELL_FROG_KISS = 62536
local TEXT_EMOTE_KISS = 58
local ENTRY_MAIDEN_OF_ASHWOOD_LAKE = 33220
local SPELL_MAIDEN_TRANSFORM = 62550
local SAY_MAIDEN_0 = 0
local SAY_MAIDEN_1 = 1

local timers = {}
local frogKissLatched = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function lakeFrogSpawn(_, creature)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
end

local function lakeFrogGossipSelect(_, player, creature)
    creature:CastSpell(player, SPELL_SUMMON_ASHWOOD_BRAND)
    return false
end

-- C++ UpdateAI :699-701: the kissed follower despawns once SPELL_FROG_LOVE
-- fades. Rechecked until the aura is gone (C++ polls every UpdateAI tick).
local function frogLoveWatch(creature, guid)
    if creature:HasAura(SPELL_FROG_LOVE) then
        schedule(guid, "fadinglove", 61000, function()
            frogLoveWatch(creature, guid)
        end)
    else
        timers[guid] = nil
        frogKissLatched[guid] = nil
        creature:Despawn()
    end
end

-- C++ EVENT_LAKEFROG_1..5 chain (zone_grizzly_hills.cpp:721-743),
-- defined tail-first so no forward declaration is needed. Cadence:
-- 1 transform + entry change 2s after the kiss, 2 Talk 2s later,
-- 3 gossip flag 3s later, 4 flag removed 25s later, 5 Talk 2s later
-- + despawn 4s after that Talk.
local function lakeFrogChain5(creature, guid)
    creature:Talk(SAY_MAIDEN_1)
    schedule(guid, "lakefrogdespawn", 4000, function()
        timers[guid] = nil
        frogKissLatched[guid] = nil
        creature:Despawn()
    end)
end

local function lakeFrogChain4(creature, guid)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    schedule(guid, "lakefrog5", 2000, function()
        lakeFrogChain5(creature, guid)
    end)
end

local function lakeFrogChain3(creature, guid)
    creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    schedule(guid, "lakefrog4", 25000, function()
        lakeFrogChain4(creature, guid)
    end)
end

local function lakeFrogChain2(creature, guid)
    creature:Talk(SAY_MAIDEN_0)
    schedule(guid, "lakefrog3", 3000, function()
        lakeFrogChain3(creature, guid)
    end)
end

local function lakeFrogChain1(creature, guid)
    creature:CastSpell(creature, SPELL_MAIDEN_TRANSFORM)
    creature:UpdateEntry(ENTRY_MAIDEN_OF_ASHWOOD_LAKE)
    schedule(guid, "lakefrog2", 2000, function()
        lakeFrogChain2(creature, guid)
    end)
end

local function lakeFrogReceiveEmote(_, creature, player, textEmote)
    local guid = creature:GetGUIDLow()
    if frogKissLatched[guid] then
        return
    end
    -- C++ IsWithinDistInMap(player, 30) compares raw 3D dist against
    -- 30 + own reach + target reach (Object.cpp _IsWithinDist); Go's
    -- IsWithinDist is raw, so compare reach-adjusted dist (target
    -- reach unmodeled, ~1.5yd band; same-map gate vacuous here).
    if textEmote == TEXT_EMOTE_KISS and creature:GetDistance(player) - creature:GetCombatReach() < 30
        and player:HasItem(ITEM_WARTS_B_GONE_LIP_BALM, 1) then
        if not player:HasAura(SPELL_WARTSBGONE_LIP_BALM) then
            player:AddAura(SPELL_WARTS)
        else
            creature:CastSpell(player, SPELL_FROG_KISS)
            if creature:GetEntry() == ENTRY_LAKE_FROG then
                creature:AddAura(SPELL_FROG_LOVE)
                schedule(guid, "fadinglove", 61000, function()
                    frogLoveWatch(creature, guid)
                end)
            elseif creature:GetEntry() == ENTRY_LAKE_FROG_QUEST then
                -- C++ :766-775 quest-frog branch: MoveIdle is vacuous
                -- (no movement model — the frog stands still) +
                -- SetFacingToObject(player) via the atan2 + SetFacingTo
                -- twin (zuljin/devourer precedent), then the
                -- EVENT_LAKEFROG_1 chain 2s later (defined above).
                local dx = player:GetX() - creature:GetX()
                local dy = player:GetY() - creature:GetY()
                local face = math.atan2(dy, dx)
                if face < 0 then face = face + 2 * math.pi end
                creature:SetFacingTo(face)
                schedule(guid, "lakefrog1", 2000, function()
                    lakeFrogChain1(creature, guid)
                end)
            end
            frogKissLatched[guid] = true
        end
    end
end

-- Pure timer/latch hygiene (no C++ JustDied/Reset counterpart for these
-- legs): cancel the fading-love watch and clear the kiss latch.
local function lakeFrogDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    frogKissLatched[guid] = nil
end

local function lakeFrogReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    frogKissLatched[guid] = nil
end

RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 5, lakeFrogSpawn)
RegisterCreatureGossipEvent(ENTRY_LAKE_FROG, 2, lakeFrogGossipSelect)
RegisterCreatureGossipEvent(ENTRY_LAKE_FROG_QUEST, 2, lakeFrogGossipSelect)
-- The maiden is the same AI after UpdateEntry (C++ SetEntry keeps the
-- object), so its gossip select + died/reset handlers ride the same
-- functions; the died/reset pair also cancels a mid-chain kill's
-- pending timers instead of casting on a dead object.
RegisterCreatureGossipEvent(ENTRY_MAIDEN_OF_ASHWOOD_LAKE, 2, lakeFrogGossipSelect)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_ASHWOOD_LAKE, 4, lakeFrogDied)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_ASHWOOD_LAKE, 23, lakeFrogReset)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 8, lakeFrogReceiveEmote)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 8, lakeFrogReceiveEmote)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 4, lakeFrogDied)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 4, lakeFrogDied)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 23, lakeFrogReset)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 23, lakeFrogReset)

-- npc_tallhorn_stagAI::Reset -> Initialize (_phase = 1); the first
-- UpdateAI tick then evaluates the _phase arm once (:373-388).
local function tallhornStagReset(_, creature)
    if creature:FindNearestGameObject(OBJECT_HAUNCH, 2.0) ~= nil then
        creature:SetStandState(UNIT_STAND_STATE_DEAD)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:SetFlag(UNIT_DYNAMIC_FLAGS_FIELD, UNIT_DYNFLAG_DEAD)
    end
end

RegisterCreatureEvent(ENTRY_TALLHORN_STAG, 5, tallhornStagReset)
RegisterCreatureEvent(ENTRY_TALLHORN_STAG, 23, tallhornStagReset)
