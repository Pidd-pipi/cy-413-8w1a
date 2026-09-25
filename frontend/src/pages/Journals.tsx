import {Button,Card,Checkbox,Col,Form,Input,Row,Select,Slider,Tag,Typography,message} from 'antd';
import dayjs from 'dayjs';
import {useEffect,useState} from 'react';
import {createJournal,deleteJournal,listJournals,updateJournal} from '../api/journal';
import {listMoods} from '../api/mood';
import {EmptyState} from '../components/common/EmptyState';
import {MoodCard} from '../components/common/MoodCard';
import {MoodSelector} from '../components/common/MoodSelector';
import type {JournalPayload,JournalView,Mood,MoodTag} from '../types';

function parseJournalTags(j:JournalView):MoodTag[]{
	try{
		const tags=JSON.parse(j.mood_tags||'[]');
		return Array.isArray(tags)&&tags.length?tags:['calm'];
	}catch{return ['calm']}
}

export function Journals(){
	const [items,setItems]=useState<JournalView[]>([]);
	const [moods,setMoods]=useState<Mood[]>([]);
	const [tags,setTags]=useState<MoodTag[]>(['calm']);
	const [editingId,setEditingId]=useState<number|null>(null);
	const [form]=Form.useForm();
	const load=()=>Promise.all([listJournals(),listMoods()]).then(([j,m])=>{setItems(j);setMoods(m)}).catch(e=>message.error(e.message));
	useEffect(()=>{load()},[]);

	const resetForm=()=>{form.resetFields();setTags(['calm']);setEditingId(null)};
	const submit=async(v:Omit<JournalPayload,'mood_tags'>)=>{
		const payload:JournalPayload={...v,mood_tags:tags};
		try{
			if(editingId){
				await updateJournal(editingId,payload);
				message.success('日记已更新，关联情绪已同步');
			}else{
				const created=await createJournal(payload);
				message.success(created.mood?'日记已保存，并生成了当天的情绪记录':'日记已保存；当天已有情绪记录，已原样保留');
			}
			resetForm();load();
		}catch(e){message.error((e as Error).message)}
	};
	const startEdit=(j:JournalView)=>{
		setEditingId(j.id);setTags(parseJournalTags(j));
		form.setFieldsValue({title:j.title,content:j.content,mood_level:j.mood_level||7,weather:j.weather||'晴',is_private:j.is_private});
		window.scrollTo({top:0,behavior:'smooth'});
	};
	const remove=async(id:number)=>{
		try{await deleteJournal(id);message.success('日记已删除，情绪记录仍保留');if(editingId===id)resetForm();load()}
		catch(e){message.error((e as Error).message)}
	};

	const sameDayMood=(j:JournalView)=>{
		const day=dayjs(j.created_at).format('YYYY-MM-DD');
		return moods.find(m=>m.journal_id!==j.id&&dayjs(m.record_date).format('YYYY-MM-DD')===day);
	};

	return <><Typography.Title>日记本</Typography.Title><Row gutter={[20,20]}><Col xs={24} lg={10}><Card title={editingId?'编辑这页日记':'写一页日记'}><Form form={form} layout="vertical" initialValues={{mood_level:7,is_private:true,weather:'晴'}} onFinish={submit}>
		<Form.Item name="title" label="标题" rules={[{required:true}]}><Input placeholder="给今天一句标题"/></Form.Item>
		<Form.Item name="content" label="正文（支持 Markdown 风格文本）" rules={[{required:true}]}><Input.TextArea rows={8} placeholder="今天发生了什么？"/></Form.Item>
		<Form.Item name="mood_level" label="心情指数（保存新日记时，若当天还没有情绪记录，会用它生成一条）"><Slider min={1} max={10}/></Form.Item>
		<Form.Item label="情绪标签（可多选）"><MoodSelector value={tags} onChange={setTags}/></Form.Item>
		<Form.Item name="weather" label="天气"><Select options={['晴','阴','雨','风'].map(v=>({value:v}))}/></Form.Item>
		<Form.Item name="is_private" valuePropName="checked"><Checkbox>仅自己可见</Checkbox></Form.Item>
		<Button type="primary" htmlType="submit">{editingId?'保存修改':'保存日记'}</Button>
		{editingId&&<Button style={{marginLeft:8}} onClick={resetForm}>取消编辑</Button>}
	</Form></Card></Col>
	<Col xs={24} lg={14}><Card title="时间轴">{items.length?<div className="timeline">{items.map(j=>{
		const manual=!j.mood&&sameDayMood(j);
		return <article className="journal" key={j.id}>
			<div><Tag color="purple">{dayjs(j.created_at).format('YYYY-MM-DD')}</Tag><b>{j.title}</b><span className="muted"> · {j.weather} · 心情 {j.mood_level}/10</span></div>
			<div style={{margin:'6px 0'}}>
				{j.mood?(j.mood.synced_from_journal?<Tag color="green">已关联情绪 · 随日记同步</Tag>:<Tag color="gold">已关联情绪 · 手动改过，保留原样</Tag>):manual?<Tag>当天已有情绪记录 · 未关联</Tag>:<Tag className="muted">无关联情绪记录</Tag>}
			</div>
			<p>{j.content}</p>
			{j.mood&&<div className="journal-mood"><MoodCard mood={j.mood}/></div>}
			<div style={{marginTop:8}}><Button size="small" onClick={()=>startEdit(j)}>编辑</Button> <Button danger size="small" onClick={()=>remove(j.id)}>删除</Button></div>
		</article>;
	})}</div>:<EmptyState title="写下第一篇日记，和自己好好聊聊"/>}</Card></Col></Row></>;
}
